// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/foundriesio/update-server/server/ui/i18n"
)

// localeUpdate stores the display language; an empty value means Automatic (follow Accept-Language).
func (h handlers) localeUpdate(c echo.Context) error {
	session := CtxGetSession(c.Request().Context())
	locale := c.Request().PostFormValue("locale")
	if locale != "" {
		if _, ok := i18n.Valid(locale); !ok {
			err := errors.New("unsupported locale")
			return EchoError(c, err, http.StatusBadRequest, err.Error())
		}
	}

	if err := session.User.SetLocale(locale, "Locale changed by "+session.User.Username); err != nil {
		return h.handleUnexpected(c, err)
	}
	return c.Redirect(http.StatusSeeOther, "/settings")
}
