// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"encoding/json"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"
)

// cssColor allows hex, rgb()/rgba()/hsl() functional notation, and CSS named
// colors — i.e. the characters those need. Anything else (e.g. ";", "{") is
// rejected so a bad branding.json can't inject or break the stylesheet.
var cssColor = regexp.MustCompile(`^[#a-zA-Z0-9(),.%\s/-]+$`)

// faviconExts bounds the content type an operator favicon can declare. Anything
// else falls back to the embedded default.
var faviconExts = map[string]bool{".svg": true, ".ico": true, ".png": true}

// Branding holds the resolved branding values (defaults applied) used by both
// HTML templates and the templated CSS.
type Branding struct {
	Title      string // app name in topbar + <title> suffix
	Logo       string // filename under <data-dir>/branding/, "" = text brand only
	Favicon    string // favicon filename under <data-dir>/branding/, "" = built-in default
	Primary    string // --brand-primary
	Accent     string // --brand-accent
	Surface    string // --surface-1 (page background)
	SurfaceAlt string // --surface-2 (content background)
	Text       string // --text-1
	// Dark-mode palette. Built-in defaults apply when colorsDark is omitted;
	// brand primary/accent are shared across modes (not duplicated here).
	SurfaceDark    string // --surface-1 in dark
	SurfaceAltDark string // --surface-2 in dark
	TextDark       string // --text-1 in dark
}

// brandingColors is the on-disk color palette shape, reused for the light
// "colors" object and the dark "colorsDark" object. In the dark object,
// primary/accent are accepted for schema symmetry but not applied — brand
// colors are shared across modes (only surfaces + text vary by mode).
type brandingColors struct {
	Primary    string `json:"primary"`
	Accent     string `json:"accent"`
	Surface    string `json:"surface"`
	SurfaceAlt string `json:"surface-alt"`
	Text       string `json:"text"`
}

// brandingFile is the on-disk JSON shape. Empty fields (absent or "") fall back
// to the built-in defaults in Branding.
type brandingFile struct {
	Title      string         `json:"title"`
	Logo       string         `json:"logo"`
	Favicon    string         `json:"favicon"`
	Colors     brandingColors `json:"colors"`
	ColorsDark brandingColors `json:"colorsDark"`
}

func defaultBranding() Branding {
	return Branding{
		Title:          "Foundries Update Server",
		Logo:           "",
		Favicon:        "",
		Primary:        "rgb(2, 11, 64)",
		Accent:         "#a3b4ff",
		Surface:        "#f2f2f2",
		SurfaceAlt:     "#ffffff",
		Text:           "#000000",
		SurfaceDark:    "#11191f",
		SurfaceAltDark: "#1a2632",
		TextDark:       "#eef1f4",
	}
}

// LoadBranding applies any overrides in the given branding.json bytes onto the
// defaults. nil/empty input or a parse error yields the defaults unchanged.
func LoadBranding(data []byte) Branding {
	b := defaultBranding()
	if len(data) == 0 {
		return b
	}
	var f brandingFile
	if err := json.Unmarshal(data, &f); err != nil {
		slog.Warn("ignoring invalid branding.json, using defaults", "error", err)
		return b
	}
	set := func(dst *string, src string) {
		if src != "" {
			*dst = src
		}
	}
	setColor := func(dst *string, src string) {
		if src != "" && cssColor.MatchString(src) {
			*dst = src
		}
	}
	set(&b.Title, f.Title)
	set(&b.Logo, f.Logo)
	if f.Favicon != "" {
		if filepath.Base(f.Favicon) == f.Favicon && faviconExts[strings.ToLower(filepath.Ext(f.Favicon))] {
			b.Favicon = f.Favicon
		}
		// invalid (traversal or bad extension) → keep default "" (embedded favicon)
	}
	setColor(&b.Primary, f.Colors.Primary)
	setColor(&b.Accent, f.Colors.Accent)
	setColor(&b.Surface, f.Colors.Surface)
	setColor(&b.SurfaceAlt, f.Colors.SurfaceAlt)
	setColor(&b.Text, f.Colors.Text)
	setColor(&b.SurfaceDark, f.ColorsDark.Surface)
	setColor(&b.SurfaceAltDark, f.ColorsDark.SurfaceAlt)
	setColor(&b.TextDark, f.ColorsDark.Text)
	return b
}
