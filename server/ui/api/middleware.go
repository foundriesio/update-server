// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"net/http"

	"github.com/foundriesio/dg-satellite/auth"
	"github.com/foundriesio/dg-satellite/storage/users"
	"github.com/foundriesio/dg-satellite/version"
	"github.com/labstack/echo/v4"
)

const versionHeader = "x-version"

func requireScope(scope users.Scopes) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			user := CtxGetUser(c.Request().Context())
			if !user.AllowedScopes.Has(scope) {
				msg := "User missing required scope(s): " + scope.String()
				return c.String(http.StatusForbidden, msg)
			}
			return next(c)
		}
	}
}

func authUser(provider auth.Provider) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			user, err := provider.GetUser(c)
			if user == nil || err != nil {
				return err
			}

			req := c.Request()
			ctx := req.Context()
			log := CtxGetLog(ctx).With("user", user.Username)
			ctx = CtxWithLog(ctx, log)
			ctx = CtxWithUser(ctx, user)
			c.SetRequest(req.WithContext(ctx))

			c.Response().Header().Set(versionHeader, version.Version)

			return next(c)
		}
	}
}

func gzipContentTypeAsContentEncoding(next echo.HandlerFunc) echo.HandlerFunc {
	// An echo.decompose middleware uses a standard content-encoding header to identify if content was gzipped.
	// We also support a non-standard way to specify that in a content-type header.
	// Although we don't know what exactly was gzipped that way, a handler may still attempt to process it.
	return func(c echo.Context) error {
		h := c.Request().Header
		if h.Get(echo.HeaderContentType) == "application/gzip" {
			h.Set(echo.HeaderContentEncoding, "gzip")
		}
		return next(c)
	}
}
