// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"

	"github.com/foundriesio/update-server/auth"
	"github.com/foundriesio/update-server/storage/users"
)

func basePrinterTag(t *testing.T, session *users.User, acceptLanguage string) string {
	t.Helper()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if acceptLanguage != "" {
		req.Header.Set("Accept-Language", acceptLanguage)
	}

	if session != nil {
		req = req.WithContext(CtxWithSession(req.Context(), &auth.Session{User: session}))
	}

	c := e.NewContext(req, httptest.NewRecorder())
	return NewPageBuilder(Branding{}).Base(c, "", "").L.Tag()
}

func TestBasePrinterLocale(t *testing.T) {
	tests := []struct {
		name           string
		session        *users.User
		acceptLanguage string
		want           string
	}{
		{"stored en wins over unsupported header", &users.User{Locale: "en"}, "it", "en"},
		{"empty stored locale follows header en", &users.User{}, "en", "en"},
		{"invalid stored locale falls back to en", &users.User{Locale: "xx"}, "en", "en"},
		{"no session and no header is english", nil, "", "en"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, basePrinterTag(t, tt.session, tt.acceptLanguage))
		})
	}
}
