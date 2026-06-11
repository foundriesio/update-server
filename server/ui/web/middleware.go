// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/foundriesio/update-server/context"
	"github.com/foundriesio/update-server/storage/users"
)

func (h handlers) requireSession(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		session, err := h.provider.GetSession(c)
		if err != nil {
			return EchoError(c, err, http.StatusInternalServerError, err.Error())
		} else if session == nil {
			return nil // The provider sent the response (e.g., redirect to login)
		}

		ctx := c.Request().Context()
		log := context.CtxGetLog(ctx).With("user", session.User.Username)
		ctx = context.CtxWithLog(ctx, log)
		ctx = CtxWithSession(ctx, session)
		c.SetRequest(c.Request().WithContext(ctx))
		return next(c)
	}
}

func (h handlers) requireScope(scope users.Scopes) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			session := CtxGetSession(c.Request().Context())
			if !session.User.AllowedScopes.Has(scope) {
				err := fmt.Errorf("user missing required scope: %s", scope)
				return h.handleError(c, http.StatusForbidden, err)
			}
			return next(c)
		}
	}
}
