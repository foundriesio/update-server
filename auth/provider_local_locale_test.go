// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/foundriesio/update-server/auth"
	"github.com/foundriesio/update-server/server/ui/web"
	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/users"
)

type localAuthEnv struct {
	e    *echo.Echo
	strg *users.Storage
	p    auth.Provider
}

// newLocalAuthEnv wires the real local provider and page builder, so the
// login and password pages render exactly as the server renders them.
func newLocalAuthEnv(t *testing.T) localAuthEnv {
	t.Helper()
	tmpdir := t.TempDir()
	db, err := storage.NewDb(filepath.Join(tmpdir, "sql.db"))
	require.NoError(t, err)
	fs, err := storage.NewFs(tmpdir)
	require.NoError(t, err)
	require.NoError(t, fs.Auth.InitHmacSecret())
	strg, err := users.NewStorage(db, fs, &storage.AuthConfig{})
	require.NoError(t, err)

	cfg := &storage.AuthConfig{
		Type:   "local",
		Config: json.RawMessage(`{"PasswordAgeDays":90}`),
	}
	e := echo.New()
	p, err := auth.NewProvider(e, db, cfg, strg, web.NewPageBuilder(web.Branding{}))
	require.NoError(t, err)
	return localAuthEnv{e: e, strg: strg, p: p}
}

func TestLocalLoginBadCredentialsRendersAcceptLanguage(t *testing.T) {
	env := newLocalAuthEnv(t)

	form := url.Values{"username": {"nobody"}, "password": {"wrong"}}.Encode()
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(form))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Accept-Language", "it")
	rec := httptest.NewRecorder()
	env.e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `<html lang="it" dir="ltr">`)
	assert.Contains(t, body, "Nome utente o password non validi")
	assert.Contains(t, body, "Accedi")
	assert.NotContains(t, body, "Invalid username or password")
	assert.Contains(t, rec.Header().Values("Vary"), "Accept-Language")
}

func TestLocalPasswordExpiredPageUsesStoredLocale(t *testing.T) {
	env := newLocalAuthEnv(t)

	// A new user has PasswordTimestamp 0, which the provider treats as expired.
	u := users.User{Username: "operator", AllowedScopes: users.ScopeDevicesR}
	require.NoError(t, env.strg.Create(&u))
	require.NoError(t, u.SetLocale("it", "test setup"))
	sessionID, err := u.CreateSession("127.0.0.1", time.Now().Add(time.Hour).Unix(), u.AllowedScopes)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/devices", nil)
	req.Header.Set("Accept-Language", "en")
	req.AddCookie(&http.Cookie{Name: auth.AuthCookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	c := env.e.NewContext(req, rec)

	session, err := env.p.GetSession(c)
	require.NoError(t, err)
	assert.Nil(t, session)

	body := rec.Body.String()
	assert.Contains(t, body, `<html lang="it" dir="ltr">`)
	assert.Contains(t, body, "Cambia password")
}

// The shared password_change_script reads its messages from the form's data attributes.
func TestLocalPasswordChangeFormCarriesScriptMessages(t *testing.T) {
	tests := []struct {
		name   string
		locale string
		form   string
	}{
		{
			name: "en",
			form: `<form id="passwordChangeForm"` +
				` data-no-match="Passwords do not match."` +
				` data-pending="Changing password…"` +
				` data-mismatch="New password and confirmation do not match."` +
				` data-failed-format="Failed to change password: {error}"` +
				`>`,
		},
		{
			name:   "it",
			locale: "it",
			form: `<form id="passwordChangeForm"` +
				` data-no-match="Le password non coincidono."` +
				` data-pending="Cambio password in corso…"` +
				` data-mismatch="La nuova password e la conferma non coincidono."` +
				` data-failed-format="Modifica della password non riuscita: {error}"` +
				`>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newLocalAuthEnv(t)

			u := users.User{Username: "operator", AllowedScopes: users.ScopeDevicesR}
			require.NoError(t, env.strg.Create(&u))
			if tt.locale != "" {
				require.NoError(t, u.SetLocale(tt.locale, "test setup"))
			}
			sessionID, err := u.CreateSession("127.0.0.1", time.Now().Add(time.Hour).Unix(), u.AllowedScopes)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodGet, "/devices", nil)
			req.Header.Set("Accept-Language", "en")
			req.AddCookie(&http.Cookie{Name: auth.AuthCookieName, Value: sessionID})
			rec := httptest.NewRecorder()
			c := env.e.NewContext(req, rec)

			session, err := env.p.GetSession(c)
			require.NoError(t, err)
			assert.Nil(t, session)

			body := rec.Body.String()
			assert.Contains(t, body, tt.form)
			// The script's lookups must match the carrier rendered above.
			assert.Contains(t, body, `form.dataset.failedFormat`)
		})
	}
}
