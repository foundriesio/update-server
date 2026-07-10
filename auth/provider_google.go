// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/users"
	"github.com/labstack/echo/v4"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"gopkg.in/go-jose/go-jose.v2/jwt"
)

type authConfigGoogle struct {
	authConfigOauth2
	AllowedDomains []string
}

type googleProvider struct {
	oauth2BaseProvider

	AllowedDomains []string
}

func (p *googleProvider) Configure(e *echo.Echo, userStorage *users.Storage, cfg *storage.AuthConfig, pageCtx PageContextBuilder) error {
	var cfgGoogle authConfigGoogle
	if err := json.Unmarshal(cfg.Config, &cfgGoogle); err != nil {
		return fmt.Errorf("unable to unmarshal google config: %w", err)
	}
	p.AllowedDomains = cfgGoogle.AllowedDomains
	p.oauthConfig = &oauth2.Config{
		RedirectURL:  cfgGoogle.BaseUrl + AuthCallbackPath,
		ClientID:     cfgGoogle.ClientID,
		ClientSecret: cfgGoogle.ClientSecret,
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     google.Endpoint,
	}
	return p.configure(e, userStorage, cfg, pageCtx)
}

type googleUser struct {
	Email        string `json:"email"`
	FirstName    string `json:"given_name"`
	LastName     string `json:"family_name"`
	HostedDomain string `json:"hd"`
}

func (u googleUser) Username() string {
	parts := strings.Split(u.Email, "@")
	return parts[0]
}

func (p *googleProvider) userFromToken(c echo.Context, token *oauth2.Token) (*users.User, error) {
	idTok := token.Extra("id_token").(string)

	tok, err := jwt.ParseSigned(idTok)
	if err != nil {
		return nil, c.String(http.StatusBadRequest, fmt.Sprintf("Could not parse oauth token: %v", err))
	}

	var profile googleUser
	if err := tok.UnsafeClaimsWithoutVerification(&profile); err != nil {
		return nil, c.String(http.StatusBadRequest, fmt.Sprintf("Could not unmarshall oauth token: %v", err))
	}

	if !slices.Contains(p.AllowedDomains, profile.HostedDomain) {
		return nil, c.String(http.StatusUnauthorized, fmt.Sprintf("Unauthorized domain: %s", profile.HostedDomain))
	}

	user, err := p.users.Upsert(profile.Username(), profile.Email, p.newUserScopes)
	if err != nil {
		return nil, c.String(http.StatusInternalServerError, fmt.Sprintf("Unexpected error retrieving user: %v", err))
	}
	return user, nil
}

func init() {
	p := googleProvider{
		oauth2BaseProvider: oauth2BaseProvider{
			name:        "google",
			displayName: "Google",
		},
	}
	p.checkToken = p.userFromToken
	RegisterProvider(&p)
}
