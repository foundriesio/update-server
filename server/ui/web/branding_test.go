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
