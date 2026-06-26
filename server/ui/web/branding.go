// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"encoding/json"
	"log/slog"
	"regexp"
)

// cssColor allows hex, rgb()/rgba()/hsl() functional notation, and CSS named
// colors — i.e. the characters those need. Anything else (e.g. ";", "{") is
// rejected so a bad branding.json can't inject or break the stylesheet.
var cssColor = regexp.MustCompile(`^[#a-zA-Z0-9(),.%\s/-]+$`)

// Branding holds the resolved branding values (defaults applied) used by both
// HTML templates and the templated CSS.
type Branding struct {
	Title      string // app name in topbar + <title> suffix
	Logo       string // filename under <data-dir>/branding/, "" = text brand only
	Primary    string // --brand-primary
	Accent     string // --brand-accent
	Surface    string // --surface-1 (page background)
	SurfaceAlt string // --surface-2 (content background)
	Text       string // --text-1
}

// brandingFile is the on-disk JSON shape. Pointers distinguish "absent" (keep
// default) from "explicitly set".
type brandingFile struct {
	Title  *string `json:"title"`
	Logo   *string `json:"logo"`
	Colors struct {
		Primary    *string `json:"primary"`
		Accent     *string `json:"accent"`
		Surface    *string `json:"surface"`
		SurfaceAlt *string `json:"surface-alt"`
		Text       *string `json:"text"`
	} `json:"colors"`
}

func defaultBranding() Branding {
	return Branding{
		Title:      "Foundries Update Server",
		Logo:       "",
		Primary:    "rgb(2, 11, 64)",
		Accent:     "#a3b4ff",
		Surface:    "#f2f2f2",
		SurfaceAlt: "#ffffff",
		Text:       "#000000",
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
	set := func(dst *string, src *string) {
		if src != nil {
			*dst = *src
		}
	}
	setColor := func(dst *string, src *string) {
		if src != nil && cssColor.MatchString(*src) {
			*dst = *src
		}
	}
	set(&b.Title, f.Title)
	set(&b.Logo, f.Logo)
	setColor(&b.Primary, f.Colors.Primary)
	setColor(&b.Accent, f.Colors.Accent)
	setColor(&b.Surface, f.Colors.Surface)
	setColor(&b.SurfaceAlt, f.Colors.SurfaceAlt)
	setColor(&b.Text, f.Colors.Text)
	return b
}
