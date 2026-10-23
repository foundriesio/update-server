// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package templates

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestJSONEscapesScriptEndTag(t *testing.T) {
	tmpl, err := Templates.Clone()
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err = tmpl.New("json-xss").Parse(`<script>const data = {{json .}};</script>`)
	if err != nil {
		t.Fatal(err)
	}

	var got bytes.Buffer
	data := map[string]string{"value": `</script><script>alert("xss")</script>`}
	if err := tmpl.ExecuteTemplate(&got, "json-xss", data); err != nil {
		t.Fatal(err)
	}
	if strings.Count(got.String(), "</script>") != 1 {
		t.Fatalf("rendered template contains an injected script end tag: %s", got.String())
	}
	if !strings.Contains(got.String(), `\u003c/script\u003e`) {
		t.Fatalf("rendered template does not escape the script end tag: %s", got.String())
	}
}

func TestOtherLabels(t *testing.T) {
	got := OtherLabels(map[string]string{
		"name":  "device-alpha",
		"group": "alpha",
		"env":   "production",
		"site":  "dallas",
	})
	want := []LabelPair{
		{Key: "env", Value: "production"},
		{Key: "site", Value: "dallas"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("OtherLabels() = %#v, want %#v", got, want)
	}
}

func TestOtherLabels_OnlyNameAndGroup(t *testing.T) {
	got := OtherLabels(map[string]string{"name": "device-alpha", "group": "alpha"})
	if len(got) != 0 {
		t.Fatalf("OtherLabels() = %#v, want empty slice", got)
	}
}

func TestOtherLabels_Nil(t *testing.T) {
	got := OtherLabels(nil)
	if len(got) != 0 {
		t.Fatalf("OtherLabels(nil) = %#v, want empty slice", got)
	}
}

func TestInitials(t *testing.T) {
	got := Initials("jdoe")
	want := "J"
	if got != want {
		t.Errorf("Initials(%q) = %q, want %q", "jdoe", got, want)
	}
}

func TestInitials_Empty(t *testing.T) {
	got := Initials("")
	want := ""
	if got != want {
		t.Errorf("Initials(%q) = %q, want %q", "", got, want)
	}
}

func TestInitials_UnicodeFirstChar(t *testing.T) {
	got := Initials("åsa")
	want := "Å"
	if got != want {
		t.Errorf("Initials(%q) = %q, want %q", "åsa", got, want)
	}
}

func TestNarrowTopbarReflowsWithoutClippingBrand(t *testing.T) {
	cssBytes, err := Assets.ReadFile("style.css")
	if err != nil {
		t.Fatalf("ReadFile(style.css) error = %v", err)
	}

	css := string(cssBytes)
	want := `@media (max-width: 480px) {
    #app,
    #app.nav-collapsed {
        grid-template-rows: auto 1fr;
    }
    #topbar {
        min-height: var(--topbar-h);
        padding: 6px 8px;
        flex-wrap: wrap;
        align-content: center;
        gap: 4px 8px;
    }
    .topbar-brand {
        min-width: 0;
        flex: 1 1 auto;
        overflow: hidden;
        font-size: 1rem;
        line-height: 1.2;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
    .topbar-right {
        width: 100%;
        flex: 1 0 100%;
        justify-content: flex-end;
        gap: 16px;
        margin-left: 0;
    }
}`
	if !strings.Contains(css, want) {
		t.Error("style.css missing the narrow topbar reflow rules")
	}
}

func TestCoarsePointerShellControlsUseTouchSizedTargets(t *testing.T) {
	cssBytes, err := Assets.ReadFile("style.css")
	if err != nil {
		t.Fatalf("ReadFile(style.css) error = %v", err)
	}

	css := string(cssBytes)
	want := `@media (pointer: coarse) {
    #topbar {
        gap: 8px;
    }
    #nav-toggle,
    #theme-toggle {
        width: 44px;
        min-width: 44px;
        height: 44px;
        min-height: 44px;
    }
    .topbar-right {
        gap: 12px;
    }
    .topbar-brand {
        min-width: 0;
        flex: 1 1 auto;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
    .topbar-right > a {
        display: inline-flex;
        min-width: 44px;
        min-height: 44px;
        align-items: center;
        justify-content: center;
    }
    summary.user-badge {
        width: 44px !important;
        height: 44px !important;
    }
    .topbar-right details.dropdown > summary + ul a,
    #sidenav nav ul li a {
        display: flex;
        min-height: 44px;
        align-items: center;
    }
}`
	if !strings.Contains(css, want) {
		t.Error("style.css missing coarse-pointer shell touch targets")
	}
}
