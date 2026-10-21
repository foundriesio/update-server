// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package templates

import (
	"bytes"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

var (
	headingAuditTagRE   = regexp.MustCompile(`(?i)<(/?)([a-z][a-z0-9-]*)(?:\s+[^<>]*?)?(/?)>`)
	headingAuditClassRE = regexp.MustCompile(`(?i)\bclass\s*=\s*(?:"([^"]*)"|'([^']*)')`)
)

type headingAuditElement struct {
	name    string
	classes map[string]bool
}

type headingAuditOwner int

const (
	headingAuditStandaloneOwner headingAuditOwner = iota
	headingAuditPageGroupOwner
	headingAuditComponentGroupOwner
)

var headingAuditOwners = map[string]headingAuditOwner{
	"applied_configs_item.html":   headingAuditPageGroupOwner,
	"configs_device_item.html":    headingAuditPageGroupOwner,
	"configs_global_history.html": headingAuditPageGroupOwner,
	"configs_item.html":           headingAuditPageGroupOwner,
	"configs_list.html":           headingAuditPageGroupOwner,
	"device.html":                 headingAuditPageGroupOwner,
	"device_apps.html":            headingAuditPageGroupOwner,
	"device_apps_states.html":     headingAuditPageGroupOwner,
	"device_labels.html":          headingAuditPageGroupOwner,
	"device_test.html":            headingAuditPageGroupOwner,
	"device_tests.html":           headingAuditPageGroupOwner,
	"device_update.html":          headingAuditPageGroupOwner,
	"device_updates.html":         headingAuditStandaloneOwner,
	"devices_list.html":           headingAuditStandaloneOwner,
	"local-login.html":            headingAuditComponentGroupOwner,
	"oauth2-login.html":           headingAuditComponentGroupOwner,
	"pki.html":                    headingAuditPageGroupOwner,
	"rollout_tail.html":           headingAuditPageGroupOwner,
	"settings.html":               headingAuditPageGroupOwner,
	"update.html":                 headingAuditPageGroupOwner,
	"update_rollout.html":         headingAuditPageGroupOwner,
	"update_tail.html":            headingAuditPageGroupOwner,
	"updates.html":                headingAuditPageGroupOwner,
	"users.html":                  headingAuditPageGroupOwner,
}

func TestNonDialogPageHeadingsUseGenericContract(t *testing.T) {
	entries, err := Assets.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir(.) error = %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".html") {
			continue
		}

		asset, err := Assets.ReadFile(entry.Name())
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", entry.Name(), err)
		}

		var ancestors []headingAuditElement
		for _, match := range headingAuditTagRE.FindAllSubmatchIndex(asset, -1) {
			name := strings.ToLower(string(asset[match[4]:match[5]]))
			if match[2] != -1 && match[3] > match[2] {
				for i := len(ancestors) - 1; i >= 0; i-- {
					if ancestors[i].name == name {
						ancestors = ancestors[:i]
						break
					}
				}
				continue
			}

			classes := headingAuditClasses(asset[match[0]:match[1]])
			line := bytes.Count(asset[:match[0]], []byte("\n")) + 1
			if name == "h1" && !headingAuditHasAncestor(ancestors, "dialog") {
				headingAuditPageHeading(t, entry.Name(), line, classes, ancestors)
			}
			if (match[6] == -1 || match[7] == match[6]) && !headingAuditVoidElement(name) {
				ancestors = append(ancestors, headingAuditElement{name: name, classes: classes})
			}
		}
	}
}

func headingAuditClasses(tag []byte) map[string]bool {
	classes := make(map[string]bool)
	match := headingAuditClassRE.FindSubmatch(tag)
	if len(match) == 0 {
		return classes
	}
	classValue := match[1]
	if len(classValue) == 0 {
		classValue = match[2]
	}
	for _, class := range strings.Fields(string(classValue)) {
		classes[class] = true
	}
	return classes
}

func headingAuditHasAncestor(ancestors []headingAuditElement, name string) bool {
	for _, ancestor := range ancestors {
		if ancestor.name == name {
			return true
		}
	}
	return false
}

func headingAuditPageHeading(t *testing.T, filename string, line int, classes map[string]bool, ancestors []headingAuditElement) {
	t.Helper()
	if !classes["heading"] {
		t.Errorf("%s:%d: non-dialog h1 must use .heading", filename, line)
	}
	roles := 0
	for _, role := range []string{
		"heading--page",
		"heading--section",
		"heading--subsection",
		"heading--entity",
		"heading--panel",
	} {
		if classes[role] {
			roles++
		}
	}
	if roles != 1 || !classes["heading--page"] {
		t.Errorf("%s:%d: non-dialog h1 must use exactly one page role", filename, line)
	}

	groups := 0
	pageGroupOwner := false
	for _, ancestor := range ancestors {
		if ancestor.classes["heading-group"] {
			groups++
			pageGroupOwner = ancestor.classes["heading-group--page"]
		}
	}

	expectedOwner, ok := headingAuditOwners[filename]
	if !ok {
		t.Errorf("%s:%d: non-dialog h1 has no ownership inventory entry", filename, line)
		return
	}
	switch expectedOwner {
	case headingAuditStandaloneOwner:
		if groups != 0 {
			t.Errorf("%s:%d: non-dialog h1 must own page spacing directly", filename, line)
		}
	case headingAuditPageGroupOwner:
		if groups != 1 || !pageGroupOwner {
			t.Errorf("%s:%d: non-dialog h1 must use a page heading-group owner", filename, line)
		}
	case headingAuditComponentGroupOwner:
		if groups != 1 || pageGroupOwner {
			t.Errorf("%s:%d: non-dialog h1 must use a component-owned heading group", filename, line)
		}
	}
}

func headingAuditVoidElement(name string) bool {
	switch name {
	case "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}

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

func TestPkiRegistryStylesAreScopedAndResponsive(t *testing.T) {
	cssBytes, err := Assets.ReadFile("style.css")
	if err != nil {
		t.Fatalf("ReadFile(style.css) error = %v", err)
	}

	css := string(cssBytes)
	for _, want := range []string{
		"@layer pki-registry {",
		`.pki-registry {
    width: 100%;
    max-width: 900px;`,
		".pki-registry .pki-registry__summary",
		".pki-registry .pki-registry__record > article {",
		`.pki-registry .pki-registry__record-header {
    display: grid;
    grid-template-columns: 280px minmax(0, 1fr) 150px;
    gap: 16px;
    align-items: start;
    margin: 0;
    padding: 14px;
    border: 0;
    background: transparent;`,
		"grid-template-columns: repeat(4, minmax(0, 1fr))",
		"grid-template-columns: 280px minmax(0, 1fr) 150px",
		"grid-template-columns: 220px minmax(0, 1fr) 150px",
		"@media (max-width: 360px)",
		".pki-registry__raw[open] .pki-registry__toggle-show",
		".pki-registry__raw[open] .pki-registry__toggle-hide",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("style.css does not contain %q", want)
		}
	}
	if strings.Contains(css, ".pki-registry__raw > summary:hover") {
		t.Error("style.css must not add a hover effect to the raw certificate summary")
	}
}
