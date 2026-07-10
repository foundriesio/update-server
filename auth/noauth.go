//go:build !disable_noauth

// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package auth

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/users"
	"github.com/labstack/echo/v4"
)

type noauthProvider struct {
	users  *users.Storage
	scopes users.Scopes
}

func (noauthProvider) Name() string {
	return "noauth"
}

func (p *noauthProvider) Configure(e *echo.Echo, storage *users.Storage, authConfig *storage.AuthConfig, pageCtx PageContextBuilder) (err error) {
	p.users = storage
	p.scopes, err = users.ScopesFromSlice(authConfig.NewUserDefaultScopes)
	return
}

func (p noauthProvider) GetUser(c echo.Context) (*users.User, error) {
	user, err := p.users.Get("noauth-fake-user")
	if err != nil {
		return nil, err
	} else if user == nil {
		slog.Info("noauth: creating noauth-fake-user user")
		user = &users.User{
			Username:      "noauth-fake-user",
			AllowedScopes: p.scopes,
		}
		err = p.users.Create(user)
		if err != nil {
			return nil, err
		}
	}
	cookie, err := c.Cookie(CsrfCookieName)
	if err != nil || cookie.Value == "" {
		SetCsrfCookie(c, time.Now().Add(24*time.Hour))
	}
	return user, nil
}

func (p noauthProvider) GetSession(c echo.Context) (*Session, error) {
	user, err := p.GetUser(c)
	if err != nil {
		return nil, err
	} else if user == nil {
		return nil, nil
	}
	return &Session{
		BaseUrl: c.Scheme() + "://" + c.Request().Host,
		User:    user,
		Client:  http.DefaultClient,
	}, nil
}

func (noauthProvider) DropSession(echo.Context, *Session) {
}

func init() {
	RegisterProvider(&noauthProvider{})
}
