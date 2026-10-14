// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/foundriesio/update-server/auth"
	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/users"
)

type cssTestAuthProvider struct{}

func (cssTestAuthProvider) Name() string { return "test" }

func (cssTestAuthProvider) Configure(*echo.Echo, *users.Storage, *storage.AuthConfig, auth.PageContextBuilder) error {
	return nil
}

func (cssTestAuthProvider) GetUser(echo.Context) (*users.User, error) { return nil, nil }

func (cssTestAuthProvider) GetSession(echo.Context) (*auth.Session, error) { return nil, nil }

func (cssTestAuthProvider) DropSession(echo.Context, *auth.Session) {}

func (cssTestAuthProvider) GetRateLimiterMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc { return next }
}

func TestCSSRouteCompressesStyleWhenRequested(t *testing.T) {
	e := echo.New()
	RegisterHandlers(e, nil, cssTestAuthProvider{}, Branding{}, "", NewPageBuilder(Branding{}))

	request := httptest.NewRequest(http.MethodGet, "/css/style.css", nil)
	request.Header.Set(echo.HeaderAcceptEncoding, gzipScheme)
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, gzipScheme, response.Header().Get(echo.HeaderContentEncoding))
	assert.Contains(t, response.Header().Get(echo.HeaderVary), echo.HeaderAcceptEncoding)

	reader, err := gzip.NewReader(bytes.NewReader(response.Body.Bytes()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reader.Close()) })
	css, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Contains(t, string(css), "@import url('/css/pico_min_211.css') layer(pico);")
}

func TestCSSRouteReturnsNotModifiedWithoutCompressedBody(t *testing.T) {
	e := echo.New()
	RegisterHandlers(e, nil, cssTestAuthProvider{}, Branding{}, "", NewPageBuilder(Branding{}))

	firstRequest := httptest.NewRequest(http.MethodGet, "/css/style.css", nil)
	firstResponse := httptest.NewRecorder()
	e.ServeHTTP(firstResponse, firstRequest)

	request := httptest.NewRequest(http.MethodGet, "/css/style.css", nil)
	request.Header.Set(echo.HeaderAcceptEncoding, gzipScheme)
	request.Header.Set("If-None-Match", firstResponse.Header().Get("ETag"))
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNotModified, response.Code)
	assert.Empty(t, response.Header().Get(echo.HeaderContentEncoding))
	assert.Empty(t, response.Body.String())
}

const gzipScheme = "gzip"
