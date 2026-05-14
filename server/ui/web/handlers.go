// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"crypto/md5"
	"fmt"
	"html/template"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/foundriesio/dg-satellite/auth"
	"github.com/foundriesio/dg-satellite/server"
	"github.com/foundriesio/dg-satellite/server/ui/web/templates"
	"github.com/foundriesio/dg-satellite/storage/users"
)

type handlers struct {
	users     *users.Storage
	provider  auth.Provider
	templates *template.Template
	styleEtag string
}

var EchoError = server.EchoError

func RegisterHandlers(e *echo.Echo, storage *users.Storage, authProvider auth.Provider) {
	cssBytes, _ := templates.Assets.ReadFile("style.css")
	h := handlers{
		users:     storage,
		provider:  authProvider,
		styleEtag: fmt.Sprintf("%x", md5.Sum(cssBytes)),
		templates: templates.Templates,
	}

	e.Renderer = h

	e.GET("/", h.index, h.requireSession)
	e.GET("/css/:filename", h.css)
	e.GET("/auth/logout", h.authLogout, h.requireSession)
	e.GET("/configs", h.configsList, h.requireSession, h.requireScope(users.ScopeDevicesR))
	e.GET("/configs/device/:uuid", h.configsDeviceItem, h.requireSession, h.requireScope(users.ScopeDevicesR))
	e.GET("/configs/device/:uuid/applied", h.configsDeviceItemApplied, h.requireSession, h.requireScope(users.ScopeDevicesR))
	e.GET("/configs/group/:name", h.configsGroupItem, h.requireSession, h.requireScope(users.ScopeDevicesR))
	e.GET("/devices", h.devicesList, h.requireSession, h.requireScope(users.ScopeDevicesR))
	e.GET("/devices/:uuid", h.devicesGet, h.requireSession, h.requireScope(users.ScopeDevicesR))
	e.GET("/devices/:uuid/apps-states", h.devicesAppsStates, h.requireSession, h.requireScope(users.ScopeDevicesR))
	e.GET("/devices/:uuid/labels", h.devicesLabelsGet, h.requireSession, h.requireScope(users.ScopeDevicesR))
	e.GET("/devices/:uuid/tests", h.devicesTests, h.requireSession, h.requireScope(users.ScopeDevicesR))
	e.GET("/devices/:uuid/tests/:testid", h.devicesTestGet, h.requireSession, h.requireScope(users.ScopeDevicesR))
	e.GET("/devices/:uuid/update/:update", h.devicesUpdateGet, h.requireSession, h.requireScope(users.ScopeDevicesR))
	e.GET("/settings", h.settings, h.requireSession)
	e.GET("/updates", h.updatesList, h.requireSession, h.requireScope(users.ScopeUpdatesR))
	e.GET("/updates/:prod/:tag/:name", h.updatesGet, h.requireSession, h.requireScope(users.ScopeUpdatesR))
	e.GET("/updates/:prod/:tag/:name/tail", h.updatesTail, h.requireSession, h.requireScope(users.ScopeUpdatesR))
	e.GET("/updates/:prod/:tag/:name/rollouts/:rollout", h.updatesRollout, h.requireSession, h.requireScope(users.ScopeUpdatesR))
	e.GET("/updates/:prod/:tag/:name/rollouts/:rollout/tail", h.updatesRolloutTail, h.requireSession, h.requireScope(users.ScopeUpdatesR))
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
	NavItems  []navItem
	CsrfToken string
}

func (h handlers) baseCtx(c echo.Context, title, selected string) baseCtx {
	var csrfToken string
	if cookie, err := c.Cookie(auth.CsrfCookieName); err == nil {
		csrfToken = cookie.Value
	}
	return baseCtx{
		User:      CtxGetSession(c.Request().Context()).User,
		Title:     title,
		NavItems:  h.genNavItems(selected),
		CsrfToken: csrfToken,
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
	return h.Render(c.Response(), c.Param("filename"), nil, c)
}

func (h handlers) index(c echo.Context) error {
	return c.Redirect(http.StatusTemporaryRedirect, "/devices")
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
