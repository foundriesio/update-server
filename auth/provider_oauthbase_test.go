// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package auth

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/foundriesio/update-server/server/ui/pagectx"
	"github.com/foundriesio/update-server/server/ui/web/templates"
)

func TestOauthLoginKeepsNext(t *testing.T) {
	e := echo.New()
	p := oauth2BaseProvider{oauthConfig: &oauth2.Config{
		Endpoint: oauth2.Endpoint{AuthURL: "https://provider.example/authorize"},
	}}

	login := func(next string) *http.Cookie {
		req := httptest.NewRequest(http.MethodGet, AuthLoginPath+"?next="+next, nil)
		rec := httptest.NewRecorder()
		require.NoError(t, p.handleLogin(e.NewContext(req, rec)))
		require.Equal(t, http.StatusTemporaryRedirect, rec.Code)
		for _, c := range rec.Result().Cookies() {
			if c.Name == oauthNextCookie {
				return c
			}
		}
		t.Fatal("no next cookie")
		return nil
	}
	callbackTarget := func(cookie *http.Cookie) string {
		req := httptest.NewRequest(http.MethodGet, AuthCallbackPath, nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		return popOauthNext(e.NewContext(req, httptest.NewRecorder()))
	}

	require.Equal(t, "/auth/activate?user_code=ABCD-1234",
		callbackTarget(login("%2Fauth%2Factivate%3Fuser_code%3DABCD-1234")))
	require.Equal(t, "/", callbackTarget(login("%2F%2Fevil.example%2F")))
	require.Equal(t, "/", callbackTarget(nil))
	// A cookie edited in the browser is checked again.
	require.Equal(t, "/", callbackTarget(&http.Cookie{Name: oauthNextCookie, Value: "https%3A%2F%2Fevil.example%2F"}))
}

func TestSameSiteRedirectPageEscapes(t *testing.T) {
	page := sameSiteRedirectPage(`/x?a=1&b="><script>`)
	require.NotContains(t, page, "<script>")
	require.Contains(t, page, `url=/x?a=1&amp;b=&#34;&gt;&lt;script&gt;"`)
}

func TestOauthLoginPageLinkCarriesNext(t *testing.T) {
	ctx := struct {
		pagectx.Base
		LoginTip    string
		Name        string
		ProviderKey string
		Reason      string
		Next        string
	}{Name: "GitHub", ProviderKey: "github", Next: "/auth/activate?user_code=ABCD-1234"}
	var out bytes.Buffer
	require.NoError(t, templates.Templates.ExecuteTemplate(&out, "oauth2-login.html", ctx))
	require.Contains(t, out.String(), `href="/auth/login?next=%2fauth%2factivate%3fuser_code%3dABCD-1234"`)
}
