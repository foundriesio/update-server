// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"bytes"
	"strings"
	"testing"

	webtemplates "github.com/foundriesio/update-server/server/ui/web/templates"
	"github.com/foundriesio/update-server/storage/users"
	"github.com/stretchr/testify/assert"
)

type settingsTemplateCtx struct {
	baseCtx
	Tokens       []users.Token
	ScopesList   []string
	ScopeGroups  []settingsScopeTemplateGroup
	LocalAuth    bool
	DeviceApiUrl string
	Oauth2ApiUrl string
}

type settingsScopeTemplateGroup struct {
	Resource   string
	Operations string
	ScopeNames []string
}

func renderSettingsHTML(t *testing.T, ctx settingsTemplateCtx) string {
	t.Helper()

	var buf bytes.Buffer
	if err := webtemplates.Templates.ExecuteTemplate(&buf, "settings.html", ctx); err != nil {
		t.Fatalf("ExecuteTemplate() error = %v", err)
	}
	return buf.String()
}

func TestSettingsTemplateRendersLedgerAndPreservesActions(t *testing.T) {
	ctx := settingsTemplateCtx{
		baseCtx: baseCtx{
			Title: "Settings",
			User: &users.User{
				Username:      "operator",
				Email:         "operator@example.test",
				AllowedScopes: users.ScopeDevicesRU | users.ScopeUpdatesR,
			},
		},
		Tokens: []users.Token{{
			PublicID:    17,
			CreatedAt:   1_780_000_000,
			ExpiresAt:   1_780_086_400,
			Description: "CI release pipeline",
			Scopes:      users.ScopeDevicesRU,
		}},
		ScopesList: []string{"devices:read-update", "updates:read"},
		ScopeGroups: []settingsScopeTemplateGroup{
			{Resource: "devices", Operations: "Read or update", ScopeNames: []string{"devices:read-update"}},
			{Resource: "updates", Operations: "Read", ScopeNames: []string{"updates:read"}},
		},
		LocalAuth:    true,
		DeviceApiUrl: "https://updates.example.test/v1/devices",
		Oauth2ApiUrl: "https://updates.example.test/oauth2",
	}

	html := renderSettingsHTML(t, ctx)

	assert.Contains(t, html, `class="settings-ledger device-page-inner"`)
	assert.Contains(t, html, `<h1 class="device-identity__name">Settings</h1>`)
	assert.Contains(t, html, `class="settings-ledger__rows"`)
	assert.Contains(t, html, `operator@example.test`)
	assert.Contains(t, html, `<a class="meta-row-action" href="/users/operator/audit-log">Audit log →</a>`)
	assert.Contains(t, html, `data-action="change-password"`)
	assert.Contains(t, html, `devices:read-update`)
	assert.Contains(t, html, `updates:read`)
	assert.Contains(t, html, `data-copy="https://updates.example.test/v1/devices"`)
	assert.Contains(t, html, `data-copy="https://updates.example.test/oauth2"`)
	assert.Contains(t, html, `data-action="open-token"`)
	assert.Contains(t, html, `data-token-id="17"`)
	assert.Contains(t, html, `data-action="revoke-token"`)
	assert.Contains(t, html, `id="tokenModal"`)
	assert.Contains(t, html, `id="description" name="description" maxlength="80" autocomplete="off" required`)
	assert.Contains(t, html, `id="expires" name="expires" autocomplete="off" required`)
	assert.Contains(t, html, `id="revokeModal"`)
	assert.Contains(t, html, `id="passwordModal"`)
	assert.Contains(t, html, `/users/operator/tokens`)
	assert.Contains(t, html, `submitPasswordChange(currentPassword, newPassword, confirmPassword`)
	assert.NotContains(t, html, `onclick=`)
	assert.Contains(t, html, `<td class="settings-ledger__scope-resource">devices</td>`)
}

func TestSettingsTemplateRendersGroupedScopesAndOutlinedControls(t *testing.T) {
	html := renderSettingsHTML(t, settingsTemplateCtx{
		baseCtx:    baseCtx{Title: "Settings", User: &users.User{Username: "operator"}},
		ScopesList: []string{"devices:read-update", "future:manage-all", "malformed"},
		ScopeGroups: []settingsScopeTemplateGroup{
			{Resource: "devices", Operations: "Read or update", ScopeNames: []string{"devices:read-update"}},
			{Resource: "future", Operations: "Manage all", ScopeNames: []string{"future:manage-all"}},
			{Resource: "malformed", Operations: "Unrecognized", ScopeNames: []string{"malformed"}},
		},
	})

	assert.Contains(t, html, `<th scope="col">Resource</th><th scope="col">Operations</th><th scope="col">Scope names</th>`)
	devicesScope := `<td class="settings-ledger__scope-resource">devices</td><td>Read or update</td>` +
		`<td><ul class="settings-ledger__scope-names"><li><code>devices:read-update</code></li></ul></td>`
	assert.Contains(t, html, devicesScope)
	assert.Contains(t, html, `<td class="settings-ledger__scope-resource">future</td><td>Manage all</td><td><ul class="settings-ledger__scope-names"><li><code>future:manage-all</code></li></ul></td>`)
	assert.Contains(t, html, `<td class="settings-ledger__scope-resource">malformed</td><td>Unrecognized</td><td><ul class="settings-ledger__scope-names"><li><code>malformed</code></li></ul></td>`)
	assert.Contains(t, html, `<button class="settings-ledger__button" type="button" data-action="open-token">New token</button>`)
	assert.NotContains(t, html, `<button class="settings-ledger__button settings-ledger__button--primary" type="button" data-action="open-token">New token</button>`)
	assert.Contains(t, html, `No API tokens. Use the New token button to create one.`)
}

func TestGroupSettingsScopesPreservesRawScopeNamesAndSummarizesOperations(t *testing.T) {
	assert.Equal(t, []settingsScopeGroup{
		{
			Resource:   "devices",
			Operations: "Create, delete, read or update",
			ScopeNames: []string{"devices:read-update", "devices:create", "devices:delete"},
		},
		{
			Resource:   "updates",
			Operations: "Delete, read or update",
			ScopeNames: []string{"updates:delete", "updates:read-update"},
		},
		{
			Resource:   "future",
			Operations: "Manage all",
			ScopeNames: []string{"future:manage-all"},
		},
		{
			Resource:   "malformed",
			Operations: "Unrecognized",
			ScopeNames: []string{"malformed"},
		},
	}, groupSettingsScopes([]string{
		"devices:read-update",
		"devices:create",
		"devices:delete",
		"updates:delete",
		"updates:read-update",
		"future:manage-all",
		"malformed",
	}))
}

func TestSettingsTemplateHidesLocalPasswordDialogForExternalAuth(t *testing.T) {
	html := renderSettingsHTML(t, settingsTemplateCtx{
		baseCtx: baseCtx{
			Title: "Settings",
			User:  &users.User{Username: "operator"},
		},
		ScopesList: []string{"devices:read"},
	})

	assert.NotContains(t, html, `<button class="settings-ledger__link" type="button" data-action="change-password">`)
	assert.NotContains(t, html, `id="passwordModal"`)
}

func TestSettingsLedgerStylesAreScopedAndResponsive(t *testing.T) {
	cssBytes, err := webtemplates.Assets.ReadFile("style.css")
	if err != nil {
		t.Fatalf("ReadFile(style.css) error = %v", err)
	}

	css := string(cssBytes)
	for _, want := range []string{
		"@layer settings-ledger {",
		`.settings-ledger {
    width: 100%;
    min-width: 0;
    color: var(--text-1);
    font-size: 14px;
    line-height: 1.5;
}`,
		`.settings-ledger__section {
    min-width: 0;
}`,
		`.settings-ledger__section-head .meta-section__title {
    margin: 0;
}`,
		`.settings-ledger__rows {
    margin: 0;
}`,
		`.settings-ledger__rows > div {
    display: grid;
    grid-template-columns: 130px minmax(0, 1fr);
    gap: 12px;
    padding: 9px 0;
    border-bottom: 1px solid var(--border-1);
}`,
		`.settings-ledger__scope-table,
.settings-ledger__token-table {
    width: 100%;
    margin: 0;
    border-collapse: collapse;
    text-align: left;
}`,
		`.settings-ledger__scope-table th:first-child {
    width: 140px;
}`,
		`.settings-ledger__scope-table th:nth-child(2),
.settings-ledger__scope-table td:nth-child(2) {
    width: 220px;
}`,
		`.settings-ledger__scope-resource {
    width: 140px;
    font: 650 0.81rem var(--mono);
}`,
		`.settings-ledger__scope-table th,
.settings-ledger__scope-table td {
    vertical-align: middle;
    background: transparent;
}`,
		`.settings-ledger__scope-names > li,
.settings-ledger__scope-list > li {
    margin: 0;
    list-style: none;
}`,
		`.settings-ledger__scope-names,
.settings-ledger__scope-list {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 14px;
    margin: 0;
    padding: 0;
    color: var(--text-2);
    font-size: 12px;
    list-style: none;
}`,
		`.settings-ledger .settings-ledger__scope-scroll,
.settings-ledger .settings-ledger__token-scroll {
    width: 100%;
    max-width: 100%;
    min-width: 0;
    contain: paint;
    overflow-x: auto;`,
		`.settings-ledger code {
    padding: 0;
    background: transparent;
}`,
		`.settings-ledger__button--primary {
    border-color: var(--brand-primary);
    background: var(--brand-primary);
    color: #fff;
}`,
		`.settings-ledger__revoke:focus {
    box-shadow: none;
}`,
		`:root[data-theme="dark"] .settings-ledger__button--primary {
    border-color: var(--brand-accent);
    background: var(--brand-accent);
    color: var(--brand-primary);
}`,
		".settings-ledger .settings-ledger__dialog",
		"@media (max-width: 620px)",
		"@media (pointer: coarse)",
		"@media (prefers-reduced-motion: reduce)",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("style.css does not contain %q", want)
		}
	}
	assert.NotContains(t, css, ".settings-ledger > h1 {")
}
