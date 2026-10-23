// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package templates

import (
	"bytes"
	"reflect"
	"regexp"
	"strconv"
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
	headingAuditSectionGroupOwner
	headingAuditComponentOwner
	headingAuditComponentGroupOwner
)

type headingAuditExpectation struct {
	role           string
	owner          headingAuditOwner
	componentClass string
}

var headingAuditExpectations = map[string]headingAuditExpectation{
	"applied_configs_item.html:15":   {role: "heading--page", owner: headingAuditPageGroupOwner},
	"applied_configs_item.html:40":   {role: "heading--section", owner: headingAuditStandaloneOwner},
	"applied_configs_item.html:59":   {role: "heading--section", owner: headingAuditStandaloneOwner},
	"configs_device_item.html:15":    {role: "heading--page", owner: headingAuditPageGroupOwner},
	"configs_device_item.html:40":    {role: "heading--section", owner: headingAuditStandaloneOwner},
	"configs_device_item.html:48":    {role: "heading--section", owner: headingAuditStandaloneOwner},
	"configs_global_history.html:11": {role: "heading--page", owner: headingAuditPageGroupOwner},
	"configs_history.html:3":         {role: "heading--page", owner: headingAuditStandaloneOwner},
	"configs_item.html:2":            {role: "heading--subsection", owner: headingAuditStandaloneOwner},
	"configs_item.html:162":          {role: "heading--page", owner: headingAuditPageGroupOwner},
	"configs_item.html:167":          {role: "heading--section", owner: headingAuditStandaloneOwner},
	"configs_item.html:175":          {role: "heading--section", owner: headingAuditStandaloneOwner},
	"configs_list.html:5":            {role: "heading--page", owner: headingAuditPageGroupOwner},
	"configs_list.html:11":           {role: "heading--section", owner: headingAuditSectionGroupOwner},
	"configs_list.html:45":           {role: "heading--section", owner: headingAuditSectionGroupOwner},
	"device.html:13":                 {role: "heading--page", owner: headingAuditPageGroupOwner},
	"device.html:102":                {role: "heading--section", owner: headingAuditStandaloneOwner},
	"device.html:118":                {role: "heading--section", owner: headingAuditStandaloneOwner},
	"device.html:157":                {role: "heading--section", owner: headingAuditStandaloneOwner},
	"device_apps.html:13":            {role: "heading--page", owner: headingAuditPageGroupOwner},
	"device_apps.html:36":            {role: "heading--section", owner: headingAuditSectionGroupOwner},
	"device_apps.html:63":            {role: "heading--section", owner: headingAuditSectionGroupOwner},
	"device_apps.html:91":            {role: "heading--subsection", owner: headingAuditStandaloneOwner},
	"device_apps_states.html:15":     {role: "heading--page", owner: headingAuditPageGroupOwner},
	"device_apps_states.html:55":     {role: "heading--panel", owner: headingAuditComponentOwner, componentClass: "snapshot-list-panel"},
	"device_apps_states.html:101":    {role: "heading--entity", owner: headingAuditComponentOwner, componentClass: "app-section-header"},
	"device_auth.html:3":             {role: "heading--page", owner: headingAuditStandaloneOwner},
	"device_auth_confirm.html:3":     {role: "heading--page", owner: headingAuditStandaloneOwner},
	"device_auth_success.html:3":     {role: "heading--page", owner: headingAuditStandaloneOwner},
	"device_labels.html:14":          {role: "heading--page", owner: headingAuditPageGroupOwner},
	"device_labels.html:21":          {role: "heading--section", owner: headingAuditStandaloneOwner},
	"device_labels.html:49":          {role: "heading--section", owner: headingAuditSectionGroupOwner},
	"device_test.html:17":            {role: "heading--page", owner: headingAuditPageGroupOwner},
	"device_test.html:44":            {role: "heading--section", owner: headingAuditStandaloneOwner},
	"device_test.html:50":            {role: "heading--section", owner: headingAuditStandaloneOwner},
	"device_test.html:83":            {role: "heading--section", owner: headingAuditStandaloneOwner},
	"device_tests.html:15":           {role: "heading--page", owner: headingAuditPageGroupOwner},
	"device_update.html:17":          {role: "heading--page", owner: headingAuditPageGroupOwner},
	"device_update.html:53":          {role: "heading--section", owner: headingAuditStandaloneOwner},
	"device_updates.html:10":         {role: "heading--page", owner: headingAuditStandaloneOwner},
	"devices_list.html:3":            {role: "heading--page", owner: headingAuditStandaloneOwner},
	"error.html:3":                   {role: "heading--page", owner: headingAuditStandaloneOwner},
	"local-login.html:24":            {role: "heading--page", owner: headingAuditComponentGroupOwner},
	"local-login.html:53":            {role: "heading--section", owner: headingAuditSectionGroupOwner},
	"local-password-change.html:4":   {role: "heading--page", owner: headingAuditStandaloneOwner},
	"oauth2-login.html:24":           {role: "heading--page", owner: headingAuditComponentGroupOwner},
	"oauth2-login.html:53":           {role: "heading--section", owner: headingAuditSectionGroupOwner},
	"pki.html:4":                     {role: "heading--page", owner: headingAuditPageGroupOwner},
	"pki.html:27":                    {role: "heading--section", owner: headingAuditStandaloneOwner},
	"pki.html:35":                    {role: "heading--entity", owner: headingAuditComponentOwner, componentClass: "heading-stack"},
	"rollout_tail.html:16":           {role: "heading--page", owner: headingAuditPageGroupOwner},
	"settings.html:4":                {role: "heading--page", owner: headingAuditPageGroupOwner},
	"settings.html:9":                {role: "heading--section", owner: headingAuditSectionGroupOwner},
	"settings.html:31":               {role: "heading--section", owner: headingAuditSectionGroupOwner},
	"settings.html:50":               {role: "heading--section", owner: headingAuditSectionGroupOwner},
	"settings.html:85":               {role: "heading--section", owner: headingAuditSectionGroupOwner},
	"tail_stream.html:3":             {role: "heading--section", owner: headingAuditStandaloneOwner},
	"tail_stream.html:29":            {role: "heading--section", owner: headingAuditSectionGroupOwner},
	"update.html:14":                 {role: "heading--page", owner: headingAuditPageGroupOwner},
	"update.html:33":                 {role: "heading--section", owner: headingAuditStandaloneOwner},
	"update.html:49":                 {role: "heading--section", owner: headingAuditStandaloneOwner},
	"update.html:62":                 {role: "heading--section", owner: headingAuditStandaloneOwner},
	"update.html:101":                {role: "heading--section", owner: headingAuditStandaloneOwner},
	"update.html:127":                {role: "heading--section", owner: headingAuditStandaloneOwner},
	"update_rollout.html:16":         {role: "heading--page", owner: headingAuditPageGroupOwner},
	"update_rollout.html:46":         {role: "heading--section", owner: headingAuditStandaloneOwner},
	"update_rollout.html:59":         {role: "heading--section", owner: headingAuditStandaloneOwner},
	"update_rollout.html:116":        {role: "heading--section", owner: headingAuditStandaloneOwner},
	"update_tail.html:14":            {role: "heading--page", owner: headingAuditPageGroupOwner},
	"updates.html:6":                 {role: "heading--page", owner: headingAuditPageGroupOwner},
	"users.html:4":                   {role: "heading--page", owner: headingAuditPageGroupOwner},
	"users.html:24":                  {role: "heading--section", owner: headingAuditStandaloneOwner},
	"users.html:34":                  {role: "heading--entity", owner: headingAuditComponentOwner, componentClass: "heading-stack"},
	"users.html:101":                 {role: "heading--section", owner: headingAuditSectionGroupOwner},
}

var legacyHeadingClasses = []string{
	"device-identity__name",
	"meta-section__title",
	"meta-section__title--sub",
	"page-title",
	"entity-name",
	"login-title",
	"snapshot-list-header",
	"app-name",
}

var fullPageHeadingTemplates = []string{
	"configs_history.html",
	"device_auth.html",
	"device_auth_confirm.html",
	"device_auth_success.html",
	"error.html",
	"local-password-change.html",
}

func TestNonDialogHeadingsUseGenericContract(t *testing.T) {
	entries, err := Assets.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir(.) error = %v", err)
	}

	headingCount := 0
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
			if (name == "h1" || name == "h2" || name == "h3") && !headingAuditHasAncestor(ancestors, "dialog") {
				headingCount++
				headingAuditHeading(t, entry.Name(), line, name, classes, ancestors)
			}
			if (match[6] == -1 || match[7] == match[6]) && !headingAuditVoidElement(name) {
				ancestors = append(ancestors, headingAuditElement{name: name, classes: classes})
			}
		}
	}
	if headingCount != 74 || len(headingAuditExpectations) != headingCount {
		t.Errorf("non-dialog heading inventory = %d headings and %d entries, want 74 each", headingCount, len(headingAuditExpectations))
	}
	headingAuditLegacySelectors(t)
}

func TestFullPagesStartWithPageHeading(t *testing.T) {
	for _, filename := range fullPageHeadingTemplates {
		asset, err := Assets.ReadFile(filename)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", filename, err)
		}

		var match []byte
		var headingName string
		for _, candidate := range headingAuditTagRE.FindAll(asset, -1) {
			candidateName := strings.ToLower(string(headingAuditTagRE.FindSubmatch(candidate)[2]))
			if candidateName == "h1" || candidateName == "h2" || candidateName == "h3" {
				match = candidate
				headingName = candidateName
				break
			}
		}
		if len(match) == 0 {
			t.Errorf("%s: missing page heading", filename)
			continue
		}
		if headingName != "h1" {
			t.Errorf("%s: first heading = %s, want h1", filename, headingName)
			continue
		}
		classes := headingAuditClasses(match)
		if !classes["heading"] || !classes["heading--page"] {
			t.Errorf("%s: first heading must use .heading.heading--page", filename)
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

func headingAuditHeading(t *testing.T, filename string, line int, name string, classes map[string]bool, ancestors []headingAuditElement) {
	t.Helper()
	key := filename + ":" + strconv.Itoa(line)
	expectation, ok := headingAuditExpectations[key]
	if !ok {
		t.Errorf("%s: non-dialog %s has no ownership inventory entry", key, name)
		return
	}
	if !classes["heading"] {
		t.Errorf("%s: non-dialog %s must use .heading", key, name)
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
	if roles != 1 || !classes[expectation.role] {
		t.Errorf("%s: non-dialog %s must use exactly one %s role", key, name, expectation.role)
	}
	groups := 0
	pageGroupOwner := false
	sectionGroupOwner := false
	componentOwner := false
	for _, ancestor := range ancestors {
		if ancestor.classes["heading-group"] {
			groups++
			pageGroupOwner = ancestor.classes["heading-group--page"]
			sectionGroupOwner = ancestor.classes["heading-group--section"]
		}
		if expectation.componentClass != "" && ancestor.classes[expectation.componentClass] {
			componentOwner = true
		}
	}
	switch expectation.owner {
	case headingAuditStandaloneOwner:
		if groups != 0 {
			t.Errorf("%s: non-dialog %s must own spacing directly", key, name)
		}
	case headingAuditPageGroupOwner:
		if groups != 1 || !pageGroupOwner {
			t.Errorf("%s: non-dialog %s must use one page heading-group owner", key, name)
		}
	case headingAuditSectionGroupOwner:
		if groups != 1 || !sectionGroupOwner {
			t.Errorf("%s: non-dialog %s must use one section heading-group owner", key, name)
		}
	case headingAuditComponentOwner:
		if groups != 0 || !componentOwner {
			t.Errorf("%s: non-dialog %s must use component owner .%s", key, name, expectation.componentClass)
		}
	case headingAuditComponentGroupOwner:
		if groups != 1 || pageGroupOwner {
			t.Errorf("%s: non-dialog %s must use a component-owned heading group", key, name)
		}
	}
	for _, legacyClass := range legacyHeadingClasses {
		if classes[legacyClass] {
			t.Errorf("%s: non-dialog %s must not use legacy .%s", key, name, legacyClass)
		}
	}
}

func headingAuditLegacySelectors(t *testing.T) {
	t.Helper()
	for _, filename := range []string{"style.css", "login.css"} {
		asset, err := Assets.ReadFile(filename)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", filename, err)
		}
		for _, legacyClass := range legacyHeadingClasses {
			selector := regexp.MustCompile(`\.` + regexp.QuoteMeta(legacyClass) + `(?:[\s>+~:#.\[,{|]|$)`)
			if selector.Match(asset) {
				t.Errorf("%s: legacy heading selector .%s remains", filename, legacyClass)
			}
		}
	}
	deviceApps, err := Assets.ReadFile("device_apps.html")
	if err != nil {
		t.Fatalf("ReadFile(device_apps.html) error = %v", err)
	}
	if !bytes.Contains(deviceApps, []byte(`<span class="app-label">`)) {
		t.Error("device_apps.html: client renderer must emit .app-label")
	}
	if bytes.Contains(deviceApps, []byte(`app-name`)) {
		t.Error("device_apps.html: client renderer must not emit .app-name")
	}
	if !bytes.Contains(deviceApps, []byte(`<div class="extra-apps" id="extra-apps" hidden>`)) {
		t.Error("device_apps.html: extra-app heading and list must share an .extra-apps wrapper")
	}
	if !regexp.MustCompile(`\.device-apps \.extra-apps\s*\{[^}]*margin-block-start:\s*24px;`).Match(deviceApps) {
		t.Error("device_apps.html: .extra-apps must retain 24px block-start separation")
	}
	for _, selector := range []string{
		`.rollout-stream .rollout-stream__help`,
		`.snapshot-list-panel .heading--panel`,
		`.pki-registry .pki-registry__identity .heading--entity`,
		`.pki-registry .pki-registry__identity .heading-support`,
		`.users-directory .account-record__identity .heading--entity`,
		`.users-directory .account-record__identity .heading-support`,
	} {
		headingAuditSelectorHasNoMargin(t, "style.css", selector)
	}
}

func headingAuditSelectorHasNoMargin(t *testing.T, filename string, selector string) {
	t.Helper()
	asset, err := Assets.ReadFile(filename)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", filename, err)
	}
	block := regexp.MustCompile(regexp.QuoteMeta(selector) + `\s*\{([^}]*)\}`).FindSubmatch(asset)
	if len(block) == 0 {
		t.Errorf("%s: missing migrated selector %s", filename, selector)
		return
	}
	if regexp.MustCompile(`(?m)\bmargin(?:-[a-z]+)?\s*:`).Match(block[1]) {
		t.Errorf("%s: migrated selector %s must not declare margin", filename, selector)
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
