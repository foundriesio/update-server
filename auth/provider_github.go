// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/users"
	"github.com/labstack/echo/v4"
	"golang.org/x/oauth2"
)

type authConfigGithub struct {
	authConfigOauth2
	AllowedOrgs []string
}

type ghProvider struct {
	oauth2BaseProvider
	AllowedOrgs []string
}

func (p *ghProvider) Configure(e *echo.Echo, users *users.Storage, cfg *storage.AuthConfig, pageCtx PageContextBuilder) error {
	var cfgGithub authConfigGithub
	if err := json.Unmarshal(cfg.Config, &cfgGithub); err != nil {
		return fmt.Errorf("unable to unmarshal github config: %w", err)
	}
	p.AllowedOrgs = cfgGithub.AllowedOrgs
	p.oauthConfig = &oauth2.Config{
		RedirectURL:  cfgGithub.BaseUrl + AuthCallbackPath,
		ClientID:     cfgGithub.ClientID,
		ClientSecret: cfgGithub.ClientSecret,
		Scopes:       []string{"user:email", "read:org"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://github.com/login/oauth/authorize",
			TokenURL: "https://github.com/login/oauth/access_token",
		},
	}
	p.loginTip = fmt.Sprintf("You must grant access to one of these organizations: %s", strings.Join(p.AllowedOrgs, ", "))
	return p.configure(e, users, cfg, pageCtx)
}

type ghProfile struct {
	Login string `json:"login"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type ghOrg struct {
	Login string `json:"login"`
}

func (p *ghProvider) userFromToken(c echo.Context, token *oauth2.Token) (*users.User, error) {
	client := p.oauthConfig.Client(c.Request().Context(), token)
	resp, err := client.Get("https://api.github.com/user")
	if err != nil {
		return nil, c.String(http.StatusInternalServerError, "Unable to request user profile")
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Error("unable to close github user body", "error", err)
		}
	}()
	if resp.StatusCode != 200 {
		msg, _ := io.ReadAll(resp.Body)
		return nil, c.String(resp.StatusCode, "Unable to read user profile: "+string(msg))
	}
	var profile ghProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, c.String(resp.StatusCode, "Unable to unmarshall user profile: "+err.Error())
	}

	// Check organization membership
	resp, err = client.Get("https://api.github.com/user/orgs")
	if err != nil {
		return nil, c.String(http.StatusInternalServerError, "Unable to request user organizations")
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Error("unable to close github user organizations body", "error", err)
		}
	}()
	if resp.StatusCode != 200 {
		msg, _ := io.ReadAll(resp.Body)
		return nil, c.String(resp.StatusCode, "Unable to read user organizations: "+string(msg))
	}
	var orgs []ghOrg
	if err := json.NewDecoder(resp.Body).Decode(&orgs); err != nil {
		return nil, c.String(resp.StatusCode, "Unable to unmarshall user organizations: "+err.Error())
	}
	found := false
	for _, org := range orgs {
		if slices.Contains(p.AllowedOrgs, org.Login) {
			found = true
		}
	}
	if !found {
		return nil, c.String(http.StatusUnauthorized, "Unauthorized organization")
	}

	user, err := p.users.Upsert(profile.Login, profile.Email, p.newUserScopes)
	if err != nil {
		return nil, c.String(http.StatusInternalServerError, "Unexpected error retrieving user")
	}
	return user, nil
}

func init() {
	p := ghProvider{
		oauth2BaseProvider: oauth2BaseProvider{
			name:        "github",
			displayName: "GitHub",
		},
	}
	p.checkToken = p.userFromToken
	RegisterProvider(&p)
}
