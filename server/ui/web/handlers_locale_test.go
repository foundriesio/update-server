// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/foundriesio/update-server/auth"
	"github.com/foundriesio/update-server/context"
	"github.com/foundriesio/update-server/server/ui/web/templates"
	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/users"
)

const (
	localeTestCsrfToken  = "locale-test-csrf"
	headerAcceptLanguage = "Accept-Language"
)

// localeTestProvider serves a fixed session; a nil user means no session.
type localeTestProvider struct {
	cssTestAuthProvider
	user *users.User
}

func (p localeTestProvider) GetSession(c echo.Context) (*auth.Session, error) {
	if p.user == nil {
		return nil, c.Redirect(http.StatusFound, auth.AuthLoginPath)
	}
	return &auth.Session{User: p.user}, nil
}

func newLocaleTestStorage(t *testing.T) *users.Storage {
	t.Helper()
	tmpdir := t.TempDir()
	db, err := storage.NewDb(filepath.Join(tmpdir, "sql.db"))
	require.NoError(t, err)
	fs, err := storage.NewFs(tmpdir)
	require.NoError(t, err)
	require.NoError(t, fs.Auth.InitHmacSecret())
	strg, err := users.NewStorage(db, fs, &storage.AuthConfig{})
	require.NoError(t, err)
	return strg
}

func newLocaleTestUser(t *testing.T, strg *users.Storage, locale string) *users.User {
	t.Helper()
	u := users.User{Username: "operator", AllowedScopes: users.ScopeDevicesR}
	require.NoError(t, strg.Create(&u))
	require.NoError(t, u.SetLocale(locale, "test setup"))
	return &u
}

func newLocaleTestServer(t *testing.T, strg *users.Storage, session *users.User) *echo.Echo {
	t.Helper()
	h := handlers{
		users:     strg,
		provider:  localeTestProvider{user: session},
		templates: templates.Templates,
		pages:     NewPageBuilder(Branding{}),
	}
	e := echo.New()
	e.Renderer = h
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := context.CtxWithLog(c.Request().Context(), slog.Default())
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.Use(auth.CsrfCheck)
	e.GET("/settings", h.settings, h.requireSession)
	e.POST("/settings/locale", h.localeUpdate, h.requireSession)
	return e
}

func postLocale(t *testing.T, e *echo.Echo, locale string) *httptest.ResponseRecorder {
	t.Helper()
	return postLocaleBody(t, e, url.Values{"locale": {locale}}.Encode())
}

func postLocaleBody(t *testing.T, e *echo.Echo, body string) *httptest.ResponseRecorder {
	t.Helper()
	form := body + "&_csrf=" + localeTestCsrfToken
	req := httptest.NewRequest(http.MethodPost, "/settings/locale", strings.NewReader(form))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(&http.Cookie{Name: auth.CsrfCookieName, Value: localeTestCsrfToken})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func storedLocale(t *testing.T, strg *users.Storage) string {
	t.Helper()
	u, err := strg.Get("operator")
	require.NoError(t, err)
	return u.Locale
}

func TestLocaleUpdateStoresSupportedLocale(t *testing.T) {
	strg := newLocaleTestStorage(t)
	e := newLocaleTestServer(t, strg, newLocaleTestUser(t, strg, ""))

	rec := postLocale(t, e, "it")

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/settings", rec.Header().Get(echo.HeaderLocation))
	assert.Equal(t, "it", storedLocale(t, strg))
}

func TestLocaleUpdateEmptyResetsToAutomatic(t *testing.T) {
	strg := newLocaleTestStorage(t)
	e := newLocaleTestServer(t, strg, newLocaleTestUser(t, strg, "it"))

	rec := postLocale(t, e, "")

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Empty(t, storedLocale(t, strg))
}

func TestLocaleUpdateRejectsUnsupportedLocale(t *testing.T) {
	for _, locale := range []string{"xx", "../it", "it/../en", "IT", " it", "it,en"} {
		t.Run(locale, func(t *testing.T) {
			strg := newLocaleTestStorage(t)
			e := newLocaleTestServer(t, strg, newLocaleTestUser(t, strg, "it"))

			rec := postLocale(t, e, locale)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Equal(t, "it", storedLocale(t, strg))
		})
	}
}

func TestLocaleUpdateUsesFirstLocaleValue(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantCode   int
		wantStored string
	}{
		{name: "first valid value is stored", body: "locale=it&locale=xx", wantCode: http.StatusSeeOther, wantStored: "it"},
		{name: "first invalid value is rejected", body: "locale=xx&locale=it", wantCode: http.StatusBadRequest, wantStored: "en"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strg := newLocaleTestStorage(t)
			e := newLocaleTestServer(t, strg, newLocaleTestUser(t, strg, "en"))

			rec := postLocaleBody(t, e, tt.body)

			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Equal(t, tt.wantStored, storedLocale(t, strg))
		})
	}
}

func TestLocaleUpdateRequiresSession(t *testing.T) {
	strg := newLocaleTestStorage(t)
	newLocaleTestUser(t, strg, "it")
	e := newLocaleTestServer(t, strg, nil)

	rec := postLocale(t, e, "en")

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, auth.AuthLoginPath, rec.Header().Get(echo.HeaderLocation))
	assert.Equal(t, "it", storedLocale(t, strg))
}

func TestLocaleUpdateRequiresCsrfToken(t *testing.T) {
	strg := newLocaleTestStorage(t)
	e := newLocaleTestServer(t, strg, newLocaleTestUser(t, strg, "it"))

	missing := httptest.NewRequest(http.MethodPost, "/settings/locale", strings.NewReader("locale=en"))
	missing.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, missing)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	mismatch := httptest.NewRequest(http.MethodPost, "/settings/locale", strings.NewReader("locale=en&_csrf=wrong"))
	mismatch.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	mismatch.AddCookie(&http.Cookie{Name: auth.CsrfCookieName, Value: localeTestCsrfToken})
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, mismatch)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	assert.Equal(t, "it", storedLocale(t, strg))
}

func TestSettingsPageSelectsStoredLocale(t *testing.T) {
	tests := []struct {
		name           string
		stored         string
		acceptLanguage string
		wantSelected   string
		wantEffective  string
	}{
		{
			name:         "stored en selects English",
			stored:       "en",
			wantSelected: `<option value="en" selected>English</option>`,
		},
		{
			name:         "stored it selects Italiano",
			stored:       "it",
			wantSelected: `<option value="it" selected>Italiano</option>`,
		},
		{
			name:           "automatic selects the automatic option and names the effective language",
			stored:         "",
			acceptLanguage: "it",
			wantSelected:   `<option value="" selected>Automatica (lingua del browser)</option>`,
			wantEffective:  "Attualmente in uso: Italiano",
		},
		{
			name:           "automatic with an unsupported browser language names English",
			stored:         "",
			acceptLanguage: "xx",
			wantSelected:   `<option value="" selected>Automatic (browser language)</option>`,
			wantEffective:  "Currently using English",
		},
		{
			name:          "stale stored locale shows automatic with its caption",
			stored:        "xx",
			wantSelected:  `<option value="" selected>Automatic (browser language)</option>`,
			wantEffective: "Currently using English",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strg := newLocaleTestStorage(t)
			e := newLocaleTestServer(t, strg, newLocaleTestUser(t, strg, tt.stored))

			req := httptest.NewRequest(http.MethodGet, "/settings", nil)
			if tt.acceptLanguage != "" {
				req.Header.Set(headerAcceptLanguage, tt.acceptLanguage)
			}
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			body := rec.Body.String()
			assert.Contains(t, body, tt.wantSelected)
			assert.Contains(t, body, `<option value="it"`)
			assert.Contains(t, body, `<option value="en"`)
			// Label above the select, not beside it.
			assert.Contains(t, body, `<div class="field-row field-row--stacked">`)
			if tt.wantEffective != "" {
				assert.Contains(t, body, tt.wantEffective)
			} else {
				assert.NotContains(t, body, "Currently using")
			}
		})
	}
}
