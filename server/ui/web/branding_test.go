// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import "testing"

func TestBrandingDefaultsWhenEmpty(t *testing.T) {
	if got := LoadBranding(nil); got != defaultBranding() {
		t.Errorf("LoadBranding(nil) = %+v, want defaults %+v", got, defaultBranding())
	}
}

func TestBrandingOverrides(t *testing.T) {
	input := []byte(`{"title":"Acme","logo":"logo.svg","colors":{"primary":"#cc0000"}}`)
	b := LoadBranding(input)
	if b.Title != "Acme" {
		t.Errorf("Title = %q, want Acme", b.Title)
	}
	if b.Primary != "#cc0000" {
		t.Errorf("Primary = %q, want #cc0000", b.Primary)
	}
	if b.Logo != "logo.svg" {
		t.Errorf("Logo = %q, want logo.svg", b.Logo)
	}
	if b.Accent != "#a3b4ff" {
		t.Errorf("Accent = %q, want default", b.Accent)
	}
}

func TestBrandingInvalidJSONFallsBackToDefaults(t *testing.T) {
	if got := LoadBranding([]byte(`{not valid`)); got != defaultBranding() {
		t.Errorf("LoadBranding(invalid) = %+v, want defaults %+v", got, defaultBranding())
	}
}

func TestBrandingRejectsMaliciousColor(t *testing.T) {
	input := []byte(`{"colors":{"primary":"red; } body { display:none } :root {"}}`)
	b := LoadBranding(input)
	if b.Primary != defaultBranding().Primary {
		t.Errorf("Primary = %q, want default (malicious value rejected)", b.Primary)
	}
}

func TestBrandingAcceptsValidColors(t *testing.T) {
	input := []byte(`{"colors":{"primary":"rgb(10, 20, 30)","accent":"hsl(200 50% 50%)","text":"#abc"}}`)
	b := LoadBranding(input)
	if b.Primary != "rgb(10, 20, 30)" || b.Accent != "hsl(200 50% 50%)" || b.Text != "#abc" {
		t.Errorf("valid colors rejected: %+v", b)
	}
}

func TestBrandingFaviconValid(t *testing.T) {
	for _, name := range []string{"favicon.svg", "logo.ico", "icon.png", "ICON.PNG"} {
		b := LoadBranding([]byte(`{"favicon":"` + name + `"}`))
		if b.Favicon != name {
			t.Errorf("Favicon = %q, want %q (valid extension kept)", b.Favicon, name)
		}
	}
}

func TestBrandingFaviconRejectsBadExtension(t *testing.T) {
	b := LoadBranding([]byte(`{"favicon":"evil.txt"}`))
	if b.Favicon != "" {
		t.Errorf("Favicon = %q, want empty (bad extension rejected)", b.Favicon)
	}
}

func TestBrandingFaviconRejectsTraversal(t *testing.T) {
	b := LoadBranding([]byte(`{"favicon":"../../etc/x.svg"}`))
	if b.Favicon != "" {
		t.Errorf("Favicon = %q, want empty (traversal rejected)", b.Favicon)
	}
}

func TestBrandingFaviconDefaultsEmpty(t *testing.T) {
	if b := LoadBranding(nil); b.Favicon != "" {
		t.Errorf("Favicon = %q, want empty by default", b.Favicon)
	}
}

func TestBrandingDarkDefaults(t *testing.T) {
	b := LoadBranding(nil)
	if b.SurfaceDark != "#11191f" {
		t.Errorf("SurfaceDark = %q, want default", b.SurfaceDark)
	}
	if b.SurfaceAltDark != "#1a2632" {
		t.Errorf("SurfaceAltDark = %q, want default", b.SurfaceAltDark)
	}
	if b.TextDark != "#eef1f4" {
		t.Errorf("TextDark = %q, want default", b.TextDark)
	}
}

func TestBrandingDarkOverrides(t *testing.T) {
	input := []byte(`{"colorsDark":{"surface":"#12151c","surface-alt":"#1a1f29","text":"#e6e8ec"}}`)
	b := LoadBranding(input)
	if b.SurfaceDark != "#12151c" || b.SurfaceAltDark != "#1a1f29" || b.TextDark != "#e6e8ec" {
		t.Errorf("dark overrides not applied: %+v", b)
	}
}

func TestBrandingDarkPartialKeepsDefaults(t *testing.T) {
	b := LoadBranding([]byte(`{"colorsDark":{"surface":"#12151c"}}`))
	if b.SurfaceDark != "#12151c" {
		t.Errorf("SurfaceDark = %q, want override", b.SurfaceDark)
	}
	if b.TextDark != "#eef1f4" {
		t.Errorf("TextDark = %q, want default (unspecified)", b.TextDark)
	}
}

func TestBrandingDarkRejectsInvalidColor(t *testing.T) {
	b := LoadBranding([]byte(`{"colorsDark":{"text":"red; } body{}"}}`))
	if b.TextDark != "#eef1f4" {
		t.Errorf("TextDark = %q, want default (invalid rejected)", b.TextDark)
	}
}

func TestBrandingLightUnaffectedByDark(t *testing.T) {
	b := LoadBranding([]byte(`{"colors":{"surface":"#fafafa"}}`))
	if b.Surface != "#fafafa" {
		t.Errorf("Surface = %q, want light override", b.Surface)
	}
	if b.SurfaceDark != "#11191f" {
		t.Errorf("SurfaceDark = %q, want default when colorsDark absent", b.SurfaceDark)
	}
}
