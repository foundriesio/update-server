// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"net/http"

	"github.com/foundriesio/update-server/storage/users"
	"github.com/labstack/echo/v4"
)

func (h handlers) settings(c echo.Context) error {
	session := CtxGetSession(c.Request().Context())
	tokens, err := session.User.ListTokens()
	if err != nil {
		return h.handleUnexpected(c, err)
	}

	ctx := struct {
		baseCtx
		Tokens     []users.Token
		ScopesList []string
		LocalAuth  bool
	}{
		baseCtx:    h.baseCtx(c, "Settings", "settings"),
		Tokens:     tokens,
		ScopesList: session.User.AllowedScopes.ToSlice(),
		LocalAuth:  h.provider.Name() == "local",
	}
	return c.Render(http.StatusOK, "settings.html", ctx)
}
