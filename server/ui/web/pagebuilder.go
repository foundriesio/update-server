// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"github.com/labstack/echo/v4"

	"github.com/foundriesio/update-server/auth"
	"github.com/foundriesio/update-server/server/ui/i18n"
	"github.com/foundriesio/update-server/server/ui/pagectx"
	"github.com/foundriesio/update-server/storage/users"
	"github.com/foundriesio/update-server/version"
)

const (
	devicesLabel = "Devices"
	updatesLabel = "Updates"
	usersLabel   = "Users"
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
	var storedLocale string
	if session, ok := c.Request().Context().Value(ctxKeySession).(*auth.Session); ok && session != nil {
		user = session.User
		navItems = b.genNavItems(selected)
		// Anonymous pages (login) have no stored locale; fall back to Accept-Language.
		if user != nil {
			storedLocale = user.Locale
		}
	}
	printer := i18n.For(i18n.Resolve(storedLocale, c.Request().Header.Get("Accept-Language")))
	return pagectx.Base{
		User:      user,
		L:         printer,
		Title:     title,
		BrandName: b.branding.Title,
		LogoPath:  logoPath,
		NavItems:  navItems,
		CsrfToken: csrfToken,
		Version:   version.AssetVersion(),
	}
}

func (b PageBuilder) genNavItems(selected string) []navItem {
	navItems := []navItem{
		{Title: devicesLabel, Href: "/devices", Selected: selected == "devices"},
		{Title: "Configs", Href: "/configs", Selected: selected == "configs"},
		{Title: updatesLabel, Href: "/updates", Selected: selected == "updates"},
		{Title: usersLabel, Href: "/users", Selected: selected == "users"},
		{Title: "PKI", Href: "/pki", Selected: selected == "pki"},
	}
	return navItems
}
