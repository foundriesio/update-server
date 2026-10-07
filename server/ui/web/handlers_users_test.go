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

func renderUsersHTML(t *testing.T, ctx usersDirectoryCtx) string {
	t.Helper()
	var buf bytes.Buffer
	if err := webtemplates.Templates.ExecuteTemplate(&buf, "users.html", ctx); err != nil {
		t.Fatalf("ExecuteTemplate() error = %v", err)
	}
	return buf.String()
}

func usersCSSRule(t *testing.T, selector string) string {
	t.Helper()
	cssBytes, err := webtemplates.Assets.ReadFile("style.css")
	if err != nil {
		t.Fatalf("ReadFile(style.css) error = %v", err)
	}

	css := string(cssBytes)
	start := strings.Index(css, selector+" {")
	if start == -1 {
		t.Fatalf("missing CSS rule for %s", selector)
	}
	end := strings.Index(css[start:], "}")
	if end == -1 {
		t.Fatalf("unterminated CSS rule for %s", selector)
	}
	return css[start : start+end]
}

func TestDirectoryPermissionsForProvider(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		scopes       users.Scopes
		want         directoryPermissions
	}{
		{
			name:         "local provider can create with create scope",
			providerName: "local",
			scopes:       users.ScopeUsersC,
			want: directoryPermissions{
				IsLocalAuth:   true,
				CanCreateUser: true,
			},
		},
		{
			name:         "external provider cannot create with create scope",
			providerName: "google",
			scopes:       users.ScopeUsersC,
			want:         directoryPermissions{},
		},
		{
			name:         "management permissions follow session scopes",
			providerName: "local",
			scopes:       users.ScopeUsersRU | users.ScopeUsersD,
			want: directoryPermissions{
				IsLocalAuth:     true,
				CanManageScopes: true,
				CanDelete:       true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, directoryPermissionsFor(tt.providerName, tt.scopes))
		})
	}
}

func TestBuildUserDirectoryEntries(t *testing.T) {
	permissions := directoryPermissionsFor(
		"local",
		users.ScopeUsersC|users.ScopeUsersRU|users.ScopeUsersD,
	)
	list := []users.User{
		{
			Username:      "alice",
			Email:         "a@example.com",
			CreatedAt:     123,
			AllowedScopes: users.ScopeDevicesRU | users.ScopeUpdatesR | users.ScopeUsersC | users.ScopeUsersD,
		},
		{Username: "bob", CreatedAt: 456},
	}

	entries := buildUserDirectoryEntries(list, "alice", permissions)
	if assert.Len(t, entries, 2) {
		currentEntry := entries[0]
		assert.Equal(t, "alice", currentEntry.Username)
		assert.Equal(t, "a@example.com", currentEntry.Email)
		assert.EqualValues(t, 123, currentEntry.CreatedAt)
		assert.Equal(t, []scopeGroup{
			{Resource: "Devices", Capabilities: []string{"Read + update"}},
			{Resource: "Updates", Capabilities: []string{"Read"}},
			{Resource: "Users", Capabilities: []string{"Create", "Delete"}},
		}, currentEntry.ScopeGroups)
		assert.Equal(t, []string{"devices:read-update", "updates:read", "users:create", "users:delete"}, currentEntry.Scopes)
		assert.Equal(t, 3, currentEntry.ResourceCount)
		assert.True(t, currentEntry.IsCurrent)
		assert.True(t, currentEntry.CanManageScopes)
		assert.False(t, currentEntry.CanResetPassword)
		assert.False(t, currentEntry.CanDelete)

		otherEntry := entries[1]
		assert.Equal(t, "bob", otherEntry.Username)
		assert.EqualValues(t, 456, otherEntry.CreatedAt)
		assert.Empty(t, otherEntry.ScopeGroups)
		assert.Zero(t, otherEntry.ResourceCount)
		assert.False(t, otherEntry.IsCurrent)
		assert.True(t, otherEntry.CanManageScopes)
		assert.True(t, otherEntry.CanResetPassword)
		assert.True(t, otherEntry.CanDelete)
	}

	externalEntries := buildUserDirectoryEntries(
		list,
		"alice",
		directoryPermissionsFor("google", users.ScopeUsersRU|users.ScopeUsersD),
	)
	assert.False(t, externalEntries[1].CanResetPassword)
}

func TestGroupScopesPreservesUnknownValues(t *testing.T) {
	assert.Equal(t, []scopeGroup{
		{Resource: "Users", Capabilities: []string{"unknown-capability"}},
		{Resource: "Future", Capabilities: []string{"manage-all"}},
	}, groupScopes([]string{"future:manage-all", "users:unknown-capability", "malformed"}))
}

func TestUsersTemplateRendersIdentityDirectory(t *testing.T) {
	longUsername := strings.Repeat("long-user-", 12)
	longEmail := strings.Repeat("long-email-", 12) + "@example.test"
	ctx := usersDirectoryCtx{
		baseCtx:       baseCtx{Title: "Users", BrandName: "Update Server"},
		CanCreateUser: true,
		IsLocalAuth:   true,
		AuthProvider:  "local",
		ScopesList:    []string{"devices:read", "users:read-update"},
		Entries: []userDirectoryEntry{
			{
				Username:         "operator",
				Email:            "operator@example.test",
				CreatedAt:        1_780_000_000,
				Scopes:           []string{"devices:read", "users:read-update"},
				ScopeGroups:      []scopeGroup{{Resource: "Devices", Capabilities: []string{"Read"}}, {Resource: "Users", Capabilities: []string{"Read + update"}}},
				ResourceCount:    2,
				CanManageScopes:  true,
				CanResetPassword: true,
				CanDelete:        true,
			},
			{
				Username:        "current-user",
				CreatedAt:       1_780_000_100,
				IsCurrent:       true,
				CanManageScopes: true,
			},
			{Username: `quote"user`, CreatedAt: 1_780_000_200, ResourceCount: 1, CanManageScopes: true},
			{Username: longUsername, Email: longEmail, CreatedAt: 1_780_000_300},
		},
	}

	html := renderUsersHTML(t, ctx)
	assert.Contains(t, html, `class="users-directory"`)
	assert.Contains(t, html, `<h1>Users</h1>`)
	assert.NotContains(t, html, `<h1 class=`)
	assert.NotContains(t, html, `identity and access at a glance`)
	assert.Contains(t, html, `class="meta-section"`)
	assert.Contains(t, html, `<span class="meta-label">Total users</span>`)
	assert.Contains(t, html, `<span class="meta-label">Authentication provider</span>`)
	assert.Contains(t, html, `<span class="meta-value is-mono">4</span>`)
	assert.Contains(t, html, `<span class="meta-value is-mono">local</span>`)
	assert.Contains(t, html, `<h2 class="meta-section__title" id="directory-title">Account directory</h2>`)
	assert.Contains(t, html, `<dd>1 resource</dd>`)
	assert.Contains(t, html, `<dd>2 resources</dd>`)
	assert.NotContains(t, html, `resource area`)
	assert.Contains(t, html, `class="account-directory"`)
	assert.Contains(t, html, `class="account-record`)
	assert.Contains(t, html, `class="account-record__identity"`)
	assert.Contains(t, html, `<h3 class="entity-name users-directory__mono">`)
	assert.Contains(t, html, `class="account-record__facts"`)
	assert.Contains(t, html, `class="access-disclosure"`)
	assert.Contains(t, html, `<span class="access-disclosure__title">View access</span>`)
	assert.Contains(t, html, `<span class="toggle-show">Show</span>`)
	assert.Contains(t, html, `<span class="toggle-hide">Hide</span>`)
	assert.Contains(t, html, `class="scope-groups"`)
	assert.Contains(t, html, `class="account-record__actions"`)
	assert.Contains(t, html, `class="account-record__primary-actions"`)
	assert.Contains(t, html, `href="/users/operator/audit-log"`)
	assert.Contains(t, html, `href="/users/operator/audit-log" aria-label="Audit history for operator"`)
	assert.Contains(t, html, `aria-label="Manage access for operator"`)
	assert.Contains(t, html, `aria-label="Reset password for operator"`)
	assert.Contains(t, html, `id="user-scopes-dialog"`)
	assert.Contains(t, html, `id="user-reset-password-dialog"`)
	assert.Contains(t, html, `id="user-create-dialog"`)
	assert.Contains(t, html, `id="user-delete-dialog"`)
	assert.Contains(t, html, `Manage access`)
	assert.Contains(t, html, `Save access`)
	assert.NotContains(t, html, `Manage scopes`)
	assert.NotContains(t, html, `Save scopes`)
	assert.Contains(t, html, `class="rollout-dialog users-directory-dialog"`)
	assert.Equal(t, 3, strings.Count(html, `class="upload-dialog users-directory-dialog`))
	assert.Contains(t, html, `class="group-grid"`)
	assert.Contains(t, html, `class="group-check"`)
	assert.Contains(t, html, `<fieldset class="field-row users-directory-dialog__access-group" aria-describedby="user-scopes-description">`)
	assert.Contains(t, html, `<legend class="field-row__key">Access levels</legend>`)
	assert.Contains(t, html, `<p class="field-row__caption" id="user-scopes-description">`)
	assert.Contains(t, html, `<fieldset class="field-row users-directory-dialog__access-group" aria-describedby="user-create-scopes-description">`)
	assert.Contains(t, html, `<legend class="field-row__key">Initial access</legend>`)
	assert.Contains(t, html, `<p class="field-row__caption" id="user-create-scopes-description">`)
	assert.NotContains(t, html, `<header class="dlg-header">`)
	assert.NotContains(t, html, `<footer class="dlg-footer">`)
	assert.Contains(t, html, `aria-live="polite"`)
	assert.Contains(t, html, `autocomplete="username"`)
	assert.Contains(t, html, `data-user="operator"`)
	assert.Contains(t, html, `class="btn-delete"`)
	assert.Contains(t, html, `aria-label="Delete user operator"`)
	assert.Contains(t, html, `<i class="trash" aria-hidden="true"></i>`)
	assert.NotContains(t, html, `class="danger-action"`)
	assert.Contains(t, html, `data-user="quote&#34;user"`)
	assert.Contains(t, html, longUsername)
	assert.Contains(t, html, longEmail)
	assert.Contains(t, html, `Email not available`)
	assert.Contains(t, html, `Current account`)
	assert.NotContains(t, html, `onclick=`)
	assert.NotContains(t, html, `alert(`)
	assert.NotContains(t, html, `confirm(`)
}

func TestUsersTemplateHidesLocalOnlyAndSelfDestructiveActions(t *testing.T) {
	ctx := usersDirectoryCtx{
		baseCtx:       baseCtx{Title: "Users", BrandName: "Update Server"},
		CanCreateUser: false,
		IsLocalAuth:   false,
		AuthProvider:  "google",
		Entries: []userDirectoryEntry{
			{
				Username:         "current-user",
				CreatedAt:        1_780_000_000,
				IsCurrent:        true,
				CanManageScopes:  true,
				CanResetPassword: true,
				CanDelete:        true,
			},
		},
	}

	html := renderUsersHTML(t, ctx)
	assert.Contains(t, html, `<span class="meta-label">Authentication provider</span>`)
	assert.Contains(t, html, `class="meta-value is-mono">google</span>`)
	assert.Contains(t, html, `data-action="manage-scopes"`)
	assert.NotContains(t, html, `Create user`)
	assert.NotContains(t, html, `id="user-create-dialog"`)
	assert.NotContains(t, html, `data-action="reset-password"`)
	assert.NotContains(t, html, `id="user-reset-password-dialog"`)
	assert.NotContains(t, html, `data-action="delete-user"`)
}

func TestUsersTemplateRendersEmptyState(t *testing.T) {
	html := renderUsersHTML(t, usersDirectoryCtx{
		baseCtx:      baseCtx{Title: "Users", BrandName: "Update Server"},
		IsLocalAuth:  true,
		AuthProvider: "local",
	})

	assert.Contains(t, html, `No users in this directory`)
	assert.NotContains(t, html, `<ol class="account-directory"`)
	assert.NotContains(t, html, `Create user`)

	html = renderUsersHTML(t, usersDirectoryCtx{
		baseCtx:       baseCtx{Title: "Users", BrandName: "Update Server"},
		CanCreateUser: true,
		IsLocalAuth:   true,
		AuthProvider:  "local",
	})
	assert.Contains(t, html, `Create user`)
}

func TestUsersDirectoryUsesSingleHeaderDivider(t *testing.T) {
	assert.Contains(t, usersCSSRule(t, ".users-directory .account-record__header"), "border-bottom: 0;")
	assert.Contains(t, usersCSSRule(t, ".users-directory .access-disclosure"), "border-top: 1px solid var(--border-1);")
}

func TestUsersDirectoryAccessContentHasTopSpacing(t *testing.T) {
	assert.Contains(t, usersCSSRule(t, ".users-directory .scope-groups"), "padding: 8px 16px 14px;")
	assert.Contains(t, usersCSSRule(t, ".users-directory .scope-groups__empty"), "padding: 8px 16px 14px;")
}

func TestUsersDirectoryCoarsePointerControlsUseTouchSizedTargets(t *testing.T) {
	cssBytes, err := webtemplates.Assets.ReadFile("style.css")
	if err != nil {
		t.Fatalf("ReadFile(style.css) error = %v", err)
	}

	css := string(cssBytes)
	want := `@media (pointer: coarse) {
    .users-directory .access-disclosure > summary,
    .users-directory .account-record__actions a,
    .users-directory .account-record__actions button:not(.btn-delete),
    .users-directory .account-record__actions .btn-delete,
    .users-directory > .btn-solid,
    .users-directory .users-directory__empty .btn-solid,
    .users-directory-dialog .dlg-close,
    .users-directory-dialog .dlg-btn,
    .users-directory-dialog .group-check {
        min-height: 44px;
    }
    .users-directory .account-record__actions .btn-delete,
    .users-directory-dialog .dlg-close {
        min-width: 44px;
    }
}`
	assert.Contains(t, css, want)
}

func TestUsersDialogActionsKeepIntrinsicWidth(t *testing.T) {
	assert.Contains(t, usersCSSRule(t, ".users-directory-dialog .dlg-btn"), "width: auto;")
}

func TestUserScopesDialogUsesBorderedPermissionLedger(t *testing.T) {
	assert.Contains(t, usersCSSRule(t, "#user-scopes-dialog .group-check"), "border: 1px solid var(--border-1);")
	assert.Contains(t, usersCSSRule(t, "#user-scopes-dialog .group-check:has(input:checked)"), "border-color: var(--interactive);")

	cssBytes, err := webtemplates.Assets.ReadFile("style.css")
	if err != nil {
		t.Fatalf("ReadFile(style.css) error = %v", err)
	}
	css := string(cssBytes)
	assert.NotContains(t, usersCSSRule(t, "#user-scopes-dialog .dlg-footer"), "border-top")
	assert.Contains(t, css, `@media (max-width: 640px) {
    #user-scopes-dialog .group-grid {
        grid-template-columns: 1fr;
    }
}`)
}

func TestUserScopesDialogKeepsCompactMobileMargins(t *testing.T) {
	cssBytes, err := webtemplates.Assets.ReadFile("style.css")
	if err != nil {
		t.Fatalf("ReadFile(style.css) error = %v", err)
	}
	assert.Contains(t, string(cssBytes), `@media (max-width: 480px) {
    #user-scopes-dialog > article {
        width: calc(100vw - 16px);
    }`)
}

func TestUsersDialogsFocusHeadingOnOpen(t *testing.T) {
	html := renderUsersHTML(t, usersDirectoryCtx{
		baseCtx:       baseCtx{Title: "Users", BrandName: "Update Server"},
		CanCreateUser: true,
		IsLocalAuth:   true,
		AuthProvider:  "local",
	})

	assert.Contains(t, html, `<h2 id="user-scopes-title" tabindex="-1">Manage access</h2>`)
	assert.Contains(t, html, `<h2 id="user-reset-password-title" tabindex="-1">Reset password</h2>`)
	assert.Contains(t, html, `<h2 id="user-create-title" tabindex="-1">Create user</h2>`)
	assert.Contains(t, html, `<h2 id="user-delete-title" tabindex="-1">Delete user</h2>`)
	assert.Contains(t, html, `var dialogTitle = dialog.querySelector('.dlg-header h2');`)
	assert.Contains(t, html, `if (dialogTitle) dialogTitle.focus();`)
	assert.NotContains(t, html, `var firstControl = dialogFocusableElements(dialog)[0];`)
}

func TestUserScopesDialogValidatesSelectionAndMatchesRolloutExitBehavior(t *testing.T) {
	html := renderUsersHTML(t, usersDirectoryCtx{
		baseCtx:     baseCtx{Title: "Users", BrandName: "Update Server"},
		ScopesList:  []string{"devices:read", "users:read-update"},
		IsLocalAuth: true,
	})

	assert.Contains(t, html, `id="user-scopes-close"`)
	assert.Contains(t, html, `id="user-scopes-cancel"`)
	assert.Contains(t, html, `data-dialog-status aria-live="polite" role="status" tabindex="-1"`)
	footerIndex := strings.Index(html, `<div class="dlg-footer">`)
	statusIndex := strings.Index(html, `class="users-directory-dialog__status"`)
	cancelIndex := strings.Index(html, `id="user-scopes-cancel"`)
	assert.Less(t, footerIndex, statusIndex)
	assert.Less(t, statusIndex, cancelIndex)
	assert.Contains(t, usersCSSRule(t, "#user-scopes-dialog .users-directory-dialog__status"), "flex: 1 1 auto;")
	assert.Contains(t, usersCSSRule(t, "#user-scopes-dialog .users-directory-dialog__status"), "margin: 0;")
	assert.Contains(t, html, `Select at least one permission before saving access.`)
	assert.Contains(t, html, `scopesSubmit.textContent = submitting ? 'Saving access…' : 'Save access';`)
	assert.Contains(t, html, `(state === 'error' || state === 'saving') && status.hasAttribute('tabindex')`)
	assert.Contains(t, html, `scopeDialog.addEventListener('cancel', function (event) {`)
	assert.Contains(t, html, `scopeDialog.addEventListener('click', function (event) {`)
	assert.Contains(t, html, `if (event.target === scopeDialog) scopeDialog.close();`)
	assert.Contains(t, html, `if (activeTrigger && document.contains(activeTrigger)) activeTrigger.focus();`)
	assert.Equal(t, 4, strings.Count(html, `setStatus(dialog, error.message, 'error');`))
}
