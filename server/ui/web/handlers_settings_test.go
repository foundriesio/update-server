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
	LocalAuth    bool
	DeviceApiUrl string
	Oauth2ApiUrl string
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
		ScopesList:   []string{"devices:read-update", "updates:read"},
		LocalAuth:    true,
		DeviceApiUrl: "https://updates.example.test/v1/devices",
		Oauth2ApiUrl: "https://updates.example.test/oauth2",
	}

	html := renderSettingsHTML(t, ctx)

	assert.Contains(t, html, `class="settings-ledger"`)
	assert.Contains(t, html, `<h1>Settings</h1>`)
	assert.Contains(t, html, `class="settings-ledger__rows"`)
	assert.Contains(t, html, `operator@example.test`)
	assert.Contains(t, html, `href="/users/operator/audit-log"`)
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
		".settings-ledger {",
		`.settings-ledger .settings-ledger__scope-scroll,
.settings-ledger .settings-ledger__token-scroll {
    width: 100%;
    max-width: 100%;
    min-width: 0;
    contain: paint;
    overflow-x: auto;`,
		".settings-ledger .settings-ledger__dialog",
		"@media (max-width: 620px)",
		"@media (pointer: coarse)",
		"@media (prefers-reduced-motion: reduce)",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("style.css does not contain %q", want)
		}
	}
}
