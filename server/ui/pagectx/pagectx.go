// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

// Package pagectx holds the shared "base" page context types used to render
// HTML pages that extend base.html. It lives in a dependency-free leaf package
// so both the web handlers and the auth providers can build the same base
// context without creating an import cycle.
package pagectx

import "github.com/foundriesio/update-server/storage/users"

// NavItem is a single entry in the top navigation bar.
type NavItem struct {
	Title    string
	Href     string
	Selected bool
}

// Base holds the fields every page that extends base.html needs.
type Base struct {
	User      *users.User
	Title     string
	BrandName string
	LogoPath  string
	NavItems  []NavItem
	CsrfToken string
	Version   string
}
