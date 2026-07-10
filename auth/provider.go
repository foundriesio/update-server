// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package auth

import (
	"fmt"
	"net/http"

	"github.com/foundriesio/update-server/server/ui/pagectx"
	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/users"
	"github.com/labstack/echo/v4"
)

// Session represents an authenticated web UI session.
type Session struct {
	BaseUrl string
	User    *users.User
	// Client is an HTTP client that includes the session cookie for making
	// authenticated requests against the REST api
	Client *http.Client
}

// PageContextBuilder builds the shared base.html page context. It is
// implemented by the web layer and injected into providers so that provider
// login pages are rendered from the same single source as every other page.
type PageContextBuilder interface {
	Base(c echo.Context, title, selected string) pagectx.Base
}

// Provider defines the interface that an authentication provider must implement
// to support a web server's authentication needs. This interface works for basic
// username/password authentication as well as OAuth2-based authentication.
type Provider interface {
	Name() string

	// Configure can be used to:
	//  - set up routes on the Echo instance
	//  - initialize any provider-specific settings
	Configure(e *echo.Echo, users *users.Storage, authConfig *storage.AuthConfig, pageCtx PageContextBuilder) error

	// GetUser retrieves the user based on either an API token or session cookie.
	GetUser(c echo.Context) (*users.User, error)

	// GetSession retrieves the session associated with the given context.
	GetSession(c echo.Context) (*Session, error)
	DropSession(c echo.Context, session *Session)
}

const AuthCookieName = "fioserver-session"
const AuthLoginPath = "/auth/login"
const AuthCallbackPath = "/auth/callback"

var providers map[string]Provider

func NewProvider(e *echo.Echo, db *storage.DbHandle, fs *storage.FsHandle, users *users.Storage, pageCtx PageContextBuilder) (Provider, error) {
	authConfig, err := fs.Auth.GetAuthConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get auth config: %w", err)
	}

	if provider, ok := providers[authConfig.Type]; ok {
		if err := provider.Configure(e, users, authConfig, pageCtx); err != nil {
			return nil, fmt.Errorf("failed to configure provider `%s`: %w", authConfig.Type, err)
		}
		return provider, nil
	}
	return nil, fmt.Errorf("no provider found with configured type `%s`", authConfig.Type)
}

func RegisterProvider(provider Provider) {
	if providers == nil {
		providers = make(map[string]Provider)
	}
	providers[provider.Name()] = provider
}
