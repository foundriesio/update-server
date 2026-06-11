// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"net/http"

	"github.com/foundriesio/update-server/context"
	"github.com/labstack/echo/v4"
)

func (h *handlers) handleUnexpected(c echo.Context, err error) error {
	return h.handleError(c, http.StatusInternalServerError, err)
}

func (h *handlers) handleError(c echo.Context, statusCode int, err error) error {
	log := context.CtxGetLog(c.Request().Context())
	log.Error("unexpected error", "status", statusCode, "error", err)

	ctx := struct {
		baseCtx
		Status  int
		Message string
	}{
		baseCtx: h.baseCtx(c, "Unexpected Error", ""),
		Status:  statusCode,
		Message: err.Error(),
	}
	return c.Render(statusCode, "error.html", ctx)
}
