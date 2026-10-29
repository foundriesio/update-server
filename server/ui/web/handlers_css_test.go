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

func TestCSSRouteIncludesGenericHeadingContract(t *testing.T) {
	e := echo.New()
	RegisterHandlers(e, nil, cssTestAuthProvider{}, Branding{}, "", NewPageBuilder(Branding{}))

	request := httptest.NewRequest(http.MethodGet, "/css/style.css", nil)
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	for _, expected := range []string{
		"--heading-space-page",
		"--heading-space-section",
		"--heading-space-subsection",
		"--heading-space-inner",
		".heading",
		".heading--page",
		".heading--section",
		".heading--subsection",
		".heading--entity",
		".heading--panel",
		".heading-group",
		".heading-group--page",
		".heading-group--section",
		".heading-stack",
		".heading-row",
		".heading-row--split",
		".heading-support",
	} {
		assert.Contains(t, response.Body.String(), expected)
	}
}

func TestCSSRouteKeepsUpdateDeleteButtonInsideActionColumn(t *testing.T) {
	e := echo.New()
	RegisterHandlers(e, nil, cssTestAuthProvider{}, Branding{}, "", NewPageBuilder(Branding{}))

	request := httptest.NewRequest(http.MethodGet, "/css/style.css", nil)
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), ".updates-table td.cell-delete {\n    padding: 12px 6px;\n}")
}

func TestCSSRouteIncludesConfigFileDialogContract(t *testing.T) {
	e := echo.New()
	RegisterHandlers(e, nil, cssTestAuthProvider{}, Branding{}, "", NewPageBuilder(Branding{}))

	request := httptest.NewRequest(http.MethodGet, "/css/style.css", nil)
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	css := response.Body.String()
	for _, expected := range []string{
		".config-file-dialog:modal",
		".config-file-dialog::backdrop",
		"overscroll-behavior: contain",
		".config-file-dialog > article",
		"width: min(720px, calc(100vw - 32px))",
		".config-file-dialog .config-file-grid",
		"grid-template-columns: minmax(0, 1fr) minmax(0, 1fr)",
		".config-file-dialog .field-row",
		".config-file-dialog textarea",
		"resize: vertical",
		"overflow: auto",
		".config-file-dialog input[readonly]",
		".config-file-dialog input[type=\"checkbox\"]:focus-visible",
		".config-file-dialog .dlg-close",
		"min-width: 24px",
		"min-height: 24px",
		".config-file-checkbox",
		".config-file-dialog__status",
		".config-file-dialog__status[data-state=\"error\"]",
		".config-file-dialog__status:focus-visible",
		".config-file-delete-dialog > article",
		"width: min(460px, calc(100vw - 32px))",
		".config-file-delete-dialog .dlg-btn {\n    width: auto;\n    flex: 0 0 auto;\n}",
		"background: var(--pico-primary-background)",
		"border: 1px solid var(--pico-primary-border)",
		"color: var(--pico-primary-inverse)",
		"@media (pointer: coarse)",
		"min-width: 44px",
		"min-height: 44px",
		"@media (max-width: 640px)",
		"width: calc(100vw - 16px)",
	} {
		assert.Contains(t, css, expected)
	}
}

func TestCSSRouteAlignsConfigFileIcons(t *testing.T) {
	e := echo.New()
	RegisterHandlers(e, nil, cssTestAuthProvider{}, Branding{}, "", NewPageBuilder(Branding{}))

	request := httptest.NewRequest(http.MethodGet, "/css/style.css", nil)
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	css := response.Body.String()
	for _, expected := range []string{
		".icon-view,\n.icon-delete {",
		"display: inline-flex",
		"align-items: center",
		"justify-content: center",
		"width: 24px",
		"height: 24px",
		"vertical-align: middle",
		"i.lock-closed {\n  display: inline-block;\n  width: 24px;\n  height: 24px;",
		"no-repeat center / 20px 20px",
	} {
		assert.Contains(t, css, expected)
	}
	assert.NotContains(t, css, `content: "\1F512"`)
	assert.NotContains(t, css, "filter: grayscale(100%)")
}

const gzipScheme = "gzip"
