// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"github.com/labstack/echo/v4"

	"github.com/foundriesio/update-server/auth"
	"github.com/foundriesio/update-server/server/ui/pagectx"
	"github.com/foundriesio/update-server/storage/users"
	"github.com/foundriesio/update-server/version"
)

// PageBuilder assembles the shared base.html page context (branding, nav,
// version, current user, csrf). It is the single place base page context is
// constructed, used by the web handlers as well as the auth providers' login
// pages (via the auth.PageContextBuilder interface).
type PageBuilder struct {
	branding Branding
}

// NewPageBuilder returns a PageBuilder for the given branding.
func NewPageBuilder(branding Branding) PageBuilder {
	return PageBuilder{branding: branding}
}

// Base builds the base page context. It is safe to call when there is no
// authenticated session (e.g. the login page): in that case User is nil and no
// navigation items are included.
func (b PageBuilder) Base(c echo.Context, title, selected string) pagectx.Base {
	var csrfToken string
	if cookie, err := c.Cookie(auth.CsrfCookieName); err == nil {
		csrfToken = cookie.Value
	}
	logoPath := ""
	if b.branding.Logo != "" {
		logoPath = "/branding/" + b.branding.Logo
	}
	var user *users.User
	var navItems []navItem
	if session, ok := c.Request().Context().Value(ctxKeySession).(*auth.Session); ok && session != nil {
		user = session.User
		navItems = b.genNavItems(selected)
	}
	return pagectx.Base{
		User:      user,
		Title:     title,
		BrandName: b.branding.Title,
		LogoPath:  logoPath,
		NavItems:  navItems,
		CsrfToken: csrfToken,
		Version:   version.Version,
	}
}

func (b PageBuilder) genNavItems(selected string) []navItem {
	navItems := []navItem{
		{Title: "Devices", Href: "/devices", Selected: selected == "devices"},
		{Title: "Configs", Href: "/configs", Selected: selected == "configs"},
		{Title: "Updates", Href: "/updates", Selected: selected == "updates"},
		{Title: "Users", Href: "/users", Selected: selected == "users"},
	}
	return navItems
}
