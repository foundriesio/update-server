// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/users"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	jose "gopkg.in/go-jose/go-jose.v2"
	"gopkg.in/go-jose/go-jose.v2/jwt"
)

func providerIdentityStorage(t *testing.T) *users.Storage {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.NewDb(filepath.Join(dir, "sql.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	fs, err := storage.NewFs(dir)
	require.NoError(t, err)
	require.NoError(t, fs.Auth.InitHmacSecret())
	s, err := users.NewStorage(db, fs, &storage.AuthConfig{})
	require.NoError(t, err)
	return s
}

type identityTransport func(*http.Request) (*http.Response, error)

func (f identityTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGithubAccountIdentity(t *testing.T) {
	s := providerIdentityStorage(t)
	p := ghProvider{
		oauth2BaseProvider: oauth2BaseProvider{users: s, oauthConfig: &oauth2.Config{}, newUserScopes: users.ScopeDevicesR},
		AllowedOrgs:        []string{"allowed"},
	}
	login := func(id int64, name, org string, wantStatus int) *users.User {
		t.Helper()
		client := &http.Client{Transport: identityTransport(func(r *http.Request) (*http.Response, error) {
			var payload any
			switch r.URL.String() {
			case "https://api.github.com/user":
				payload = ghProfile{ID: id, Login: name, Email: "alice@example.com"}
			case "https://api.github.com/user/orgs":
				payload = []ghOrg{{Login: org}}
			default:
				t.Errorf("unexpected URL: %s", r.URL)
			}
			body, err := json.Marshal(payload)
			require.NoError(t, err)
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body)))}, nil
		})}
		req := httptest.NewRequest(http.MethodGet, AuthCallbackPath, nil)
		req = req.WithContext(context.WithValue(req.Context(), oauth2.HTTPClient, client))
		rec := httptest.NewRecorder()
		u, err := p.userFromToken(echo.New().NewContext(req, rec), &oauth2.Token{AccessToken: "test"})
		require.NoError(t, err)
		require.Equal(t, wantStatus, rec.Code)
		return u
	}
	a := login(123, "alice", "allowed", http.StatusOK)
	require.NotNil(t, a)
	a.AllowedScopes = users.ScopeUpdatesRU
	require.NoError(t, a.Update("Change permissions"))
	b := login(123, "renamed", "allowed", http.StatusOK)
	require.Equal(t, a.Username, b.Username)
	require.Equal(t, users.ScopeUpdatesRU, b.AllowedScopes)
	c := login(456, "alice", "allowed", http.StatusOK)
	require.NotEqual(t, a.Username, c.Username)
	require.Equal(t, users.ScopeDevicesR, c.AllowedScopes)
	require.Nil(t, login(0, "alice", "allowed", http.StatusUnauthorized))
	require.Nil(t, login(-1, "alice", "allowed", http.StatusUnauthorized))
	require.Nil(t, login(123, "alice", "other", http.StatusUnauthorized))
	require.NoError(t, a.Delete())
	require.Nil(t, login(123, "renamed", "allowed", http.StatusForbidden))
}

func TestGoogleAccountIdentity(t *testing.T) {
	s := providerIdentityStorage(t)
	p := googleProvider{
		oauth2BaseProvider: oauth2BaseProvider{users: s, newUserScopes: users.ScopeDevicesR},
		AllowedDomains:     []string{"a.com", "b.com"},
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS256, Key: []byte("01234567890123456789012345678901")}, nil)
	require.NoError(t, err)
	login := func(subject, email, domain string, wantStatus int) *users.User {
		t.Helper()
		encoded, err := jwt.Signed(signer).Claims(googleUser{Subject: subject, Email: email, HostedDomain: domain}).CompactSerialize()
		require.NoError(t, err)
		token := (&oauth2.Token{}).WithExtra(map[string]any{"id_token": encoded})
		rec := httptest.NewRecorder()
		ctx := echo.New().NewContext(httptest.NewRequest(http.MethodGet, AuthCallbackPath, nil), rec)
		u, err := p.userFromToken(ctx, token)
		require.NoError(t, err)
		require.Equal(t, wantStatus, rec.Code)
		return u
	}
	a := login("123", "alice@a.com", "a.com", http.StatusOK)
	require.NotNil(t, a)
	a.AllowedScopes = users.ScopeUpdatesRU
	require.NoError(t, a.Update("Change permissions"))
	b := login("456", "alice@b.com", "b.com", http.StatusOK)
	require.NotEqual(t, a.Username, b.Username)
	require.Equal(t, users.ScopeDevicesR, b.AllowedScopes)
	renamed := login("123", "renamed@a.com", "a.com", http.StatusOK)
	require.Equal(t, a.Username, renamed.Username)
	require.Equal(t, "renamed@a.com", renamed.Email)
	require.Equal(t, users.ScopeUpdatesRU, renamed.AllowedScopes)
	reused := login("789", "alice@a.com", "a.com", http.StatusOK)
	require.NotEqual(t, a.Username, reused.Username)
	require.Equal(t, users.ScopeDevicesR, reused.AllowedScopes)
	require.Nil(t, login("", "alice@a.com", "a.com", http.StatusUnauthorized))
	require.Nil(t, login("123", "alice@other.com", "other.com", http.StatusUnauthorized))
	require.NoError(t, a.Delete())
	require.Nil(t, login("123", "renamed@a.com", "a.com", http.StatusForbidden))
	for _, extra := range []any{nil, 123, ""} {
		rec := httptest.NewRecorder()
		ctx := echo.New().NewContext(httptest.NewRequest(http.MethodGet, AuthCallbackPath, nil), rec)
		u, err := p.userFromToken(ctx, (&oauth2.Token{}).WithExtra(map[string]any{"id_token": extra}))
		require.NoError(t, err)
		require.Nil(t, u)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	}
}
