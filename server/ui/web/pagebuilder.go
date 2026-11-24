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
	var storedLocale string
	session, _ := c.Request().Context().Value(ctxKeySession).(*auth.Session)
	if session != nil {
		user = session.User
		// Anonymous pages (login) have no stored locale; fall back to Accept-Language.
		if user != nil {
			storedLocale = user.Locale
		}
	}
	printer := i18n.For(i18n.Resolve(storedLocale, c.Request().Header.Get("Accept-Language")))
	var navItems []navItem
	if session != nil {
		navItems = b.genNavItems(selected, printer)
	}
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

// Literal T calls are required: the i18nscan tool extracts messages from Go source by pattern.
func (b PageBuilder) genNavItems(selected string, L i18n.Printer) []navItem {
	navItems := []navItem{
		{Title: L.T("Devices"), Href: "/devices", Selected: selected == "devices"},
		{Title: L.T("Configs"), Href: "/configs", Selected: selected == "configs"},
		{Title: L.T("Updates"), Href: "/updates", Selected: selected == "updates"},
		{Title: L.T("Users"), Href: "/users", Selected: selected == "users"},
		{Title: L.T("PKI"), Href: "/pki", Selected: selected == "pki"},
	}
	return navItems
}
