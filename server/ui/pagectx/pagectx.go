// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

// Package pagectx holds the shared base page context for pages that extend
// base.html. It sits below both web and auth so each can build it without an
// import cycle.
package pagectx

import (
	"github.com/foundriesio/update-server/server/ui/i18n"
	"github.com/foundriesio/update-server/storage/users"
)

// NavItem is a single entry in the top navigation bar.
type NavItem struct {
	Title    string
	Href     string
	Selected bool
}

// Base holds the fields every page that extends base.html needs.
type Base struct {
	User *users.User
	// L renders translated strings and locale-aware formatting for the request.
	L         i18n.Printer
	Title     string
	BrandName string
	LogoPath  string
	NavItems  []NavItem
	CsrfToken string
	Version   string
}
