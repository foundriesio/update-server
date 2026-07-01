// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/labstack/echo/v4"

	"github.com/foundriesio/update-server/auth"
	"github.com/foundriesio/update-server/server"
	"github.com/foundriesio/update-server/server/ui/web/templates"
	"github.com/foundriesio/update-server/storage/users"
	"github.com/foundriesio/update-server/version"
)

type handlers struct {
	users       *users.Storage
	provider    auth.Provider
	templates   *template.Template
	styleEtag   string
	branding    Branding
	brandingDir string
}

var EchoError = server.EchoError

func RegisterHandlers(e *echo.Echo, storage *users.Storage, authProvider auth.Provider, branding Branding, brandingDir string) {
	h := handlers{
		users:       storage,
		provider:    authProvider,
		templates:   templates.Templates,
		branding:    branding,
		brandingDir: brandingDir,
	}
	var rendered bytes.Buffer
	if err := h.templates.ExecuteTemplate(&rendered, "style.css", branding); err != nil {
		slog.Error("failed to render style.css for etag", "error", err)
	}
	h.styleEtag = fmt.Sprintf("%x", md5.Sum(rendered.Bytes()))

	e.Renderer = h

	e.GET("/", h.index, h.requireSession)
	e.GET("/css/:filename", h.css)
	e.GET("/branding/:filename", h.brandingAsset)
	e.GET("/favicon", h.favicon)
	e.GET("/auth/logout", h.authLogout, h.requireSession)
	e.GET("/configs", h.configsList, h.requireSession, h.requireScope(users.ScopeDevicesR))
	e.GET("/configs/device/:uuid", h.configsDeviceItem, h.requireSession, h.requireScope(users.ScopeDevicesR))
	e.PATCH("/configs/device/:uuid", h.configsDeviceItemPatch, h.requireSession,
		h.requireScope(users.ScopeDevicesRU|users.ScopeUpdatesRU), auth.CsrfCheck)
	e.DELETE("/configs/device/:uuid", h.configsDeviceItemDelete, h.requireSession,
		h.requireScope(users.ScopeDevicesRU|users.ScopeUpdatesRU), auth.CsrfCheck)
	e.GET("/configs/device/:uuid/applied", h.configsDeviceItemApplied, h.requireSession, h.requireScope(users.ScopeDevicesR))
	e.GET("/configs/device/:uuid/history", h.configsDeviceItemHistory, h.requireSession, h.requireScope(users.ScopeDevicesR))
	e.PATCH("/configs/global", h.configsGlobalPatch, h.requireSession,
		h.requireScope(users.ScopeDevicesRU|users.ScopeUpdatesRU), auth.CsrfCheck)
	e.DELETE("/configs/global", h.configsGlobalDelete, h.requireSession,
		h.requireScope(users.ScopeDevicesRU|users.ScopeUpdatesRU), auth.CsrfCheck)
	e.GET("/configs/global/history", h.configsGlobalHistory, h.requireSession, h.requireScope(users.ScopeDevicesR))
	e.GET("/configs/group/:name", h.configsGroupItem, h.requireSession, h.requireScope(users.ScopeDevicesR))
	e.PATCH("/configs/group/:name", h.configsGroupItemPatch, h.requireSession,
		h.requireScope(users.ScopeDevicesRU|users.ScopeUpdatesRU), auth.CsrfCheck)
	e.DELETE("/configs/group/:name", h.configsGroupItemDelete, h.requireSession,
		h.requireScope(users.ScopeDevicesRU|users.ScopeUpdatesRU), auth.CsrfCheck)
	e.GET("/configs/group/:name/history", h.configsGroupItemHistory, h.requireSession, h.requireScope(users.ScopeDevicesR))
	e.GET("/devices", h.devicesList, h.requireSession, h.requireScope(users.ScopeDevicesR))
	e.GET("/devices/:uuid", h.devicesGet, h.requireSession, h.requireScope(users.ScopeDevicesR))
	e.GET("/devices/:uuid/apps-states", h.devicesAppsStates, h.requireSession, h.requireScope(users.ScopeDevicesR))
	e.GET("/devices/:uuid/labels", h.devicesLabelsGet, h.requireSession, h.requireScope(users.ScopeDevicesR))
	e.GET("/devices/:uuid/tests", h.devicesTests, h.requireSession, h.requireScope(users.ScopeDevicesR))
	e.GET("/devices/:uuid/tests/:testid", h.devicesTestGet, h.requireSession, h.requireScope(users.ScopeDevicesR))
	e.GET("/devices/:uuid/update/:update", h.devicesUpdateGet, h.requireSession, h.requireScope(users.ScopeDevicesR))
	e.GET("/settings", h.settings, h.requireSession)
	e.GET("/updates", h.updatesList, h.requireSession, h.requireScope(users.ScopeUpdatesR))
	e.GET("/updates/:tag/:name", h.updatesGet, h.requireSession, h.requireScope(users.ScopeUpdatesR))
	e.GET("/updates/:tag/:name/tail", h.updatesTail, h.requireSession, h.requireScope(users.ScopeUpdatesR))
	e.GET("/updates/:tag/:name/rollouts/:rollout", h.updatesRollout, h.requireSession, h.requireScope(users.ScopeUpdatesR))
	e.GET("/updates/:tag/:name/rollouts/:rollout/tail", h.updatesRolloutTail, h.requireSession, h.requireScope(users.ScopeUpdatesR))
	e.GET("/users", h.usersList, h.requireSession, h.requireScope(users.ScopeUsersR))
	e.DELETE("/users/:username", h.userDelete, h.requireSession, h.requireScope(users.ScopeUsersD))
	e.GET("/users/:username/audit-log", h.usersAuditLog, h.requireSession, h.requireScope(users.ScopeUsersR))
	e.POST("/users/:username/tokens", h.userTokenCreate, h.requireSession)
	e.PUT("/users/:username/scopes", h.userScopesUpdate, h.requireSession)
	e.DELETE("/users/:username/tokens/:tokenID", h.userTokenDelete, h.requireSession)
}

type baseCtx struct {
	User      *users.User
	Title     string
	BrandName string
	LogoPath  string
	NavItems  []navItem
	CsrfToken string
	Version   string
}

func (h handlers) baseCtx(c echo.Context, title, selected string) baseCtx {
	var csrfToken string
	if cookie, err := c.Cookie(auth.CsrfCookieName); err == nil {
		csrfToken = cookie.Value
	}
	logoPath := ""
	if h.branding.Logo != "" {
		logoPath = "/branding/" + h.branding.Logo
	}
	return baseCtx{
		User:      CtxGetSession(c.Request().Context()).User,
		Title:     title,
		BrandName: h.branding.Title,
		LogoPath:  logoPath,
		NavItems:  h.genNavItems(selected),
		CsrfToken: csrfToken,
		Version:   version.Version,
	}
}

func (h handlers) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return h.templates.ExecuteTemplate(w, name, data)
}

func (h handlers) css(c echo.Context) error {
	if eTag := c.Request().Header.Get("If-None-Match"); eTag == h.styleEtag {
		return c.NoContent(http.StatusNotModified)
	}
	c.Response().Header().Set("ETag", h.styleEtag)
	c.Response().Header().Set("Cache-Control", "public, max-age=3600") // 1 hour in seconds
	c.Response().Header().Set("Content-Type", "text/css")
	return h.Render(c.Response(), c.Param("filename"), h.branding, c)
}

func (h handlers) index(c echo.Context) error {
	return c.Redirect(http.StatusTemporaryRedirect, "/devices")
}

func (h handlers) brandingAsset(c echo.Context) error {
	logo := h.branding.Logo
	if logo == "" || c.Param("filename") != logo || filepath.Base(logo) != logo {
		return echo.ErrNotFound
	}
	return c.File(filepath.Join(h.brandingDir, logo))
}

func (h handlers) favicon(c echo.Context) error {
	// Operator override from disk. Extension + Base already validated at load
	// time; Base re-checked here as defense-in-depth (matches brandingAsset).
	if f := h.branding.Favicon; f != "" && filepath.Base(f) == f {
		p := filepath.Join(h.brandingDir, f)
		if _, err := os.Stat(p); err == nil {
			return c.File(p) // Content-Type by extension + Last-Modified from disk
		}
		// configured file missing → fall through to embedded default
	}
	// Add .ico/legacy links only if a real client needs them — modern browsers
	// all honor the SVG.
	b, err := templates.Assets.ReadFile("favicon.svg")
	if err != nil {
		return echo.ErrNotFound
	}
	c.Response().Header().Set("Cache-Control", "public, max-age=3600")
	return c.Blob(http.StatusOK, "image/svg+xml", b)
}

type navItem struct {
	Title    string
	Href     string
	Selected bool
}

func (h handlers) genNavItems(selected string) []navItem {
	navItems := []navItem{
		{Title: "Devices", Href: "/devices", Selected: selected == "devices"},
		{Title: "Configs", Href: "/configs", Selected: selected == "configs"},
		{Title: "Updates", Href: "/updates", Selected: selected == "updates"},
		{Title: "Users", Href: "/users", Selected: selected == "users"},
	}
	return navItems
}

func (h *handlers) authLogout(c echo.Context) error {
	h.provider.DropSession(c, CtxGetSession(c.Request().Context()))
	return c.Redirect(http.StatusTemporaryRedirect, "/")
}
