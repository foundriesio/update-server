// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/foundriesio/update-server/auth"
	"github.com/foundriesio/update-server/context"
	webtemplates "github.com/foundriesio/update-server/server/ui/web/templates"
	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/users"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type settingsTemplateCtx struct {
	baseCtx
	Tokens          []users.Token
	ScopesList      []string
	GrantableScopes []string
	ScopeGroups     []settingsScopeTemplateGroup
	LocalAuth       bool
	DeviceApiUrl    string
	Oauth2ApiUrl    string
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
		ScopesList:      []string{"devices:read-update", "updates:read"},
		GrantableScopes: []string{"devices:read", "devices:read-update", "updates:read"},
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
	assert.Contains(t, html, `<header class="heading-group heading-group--page">`)
	assert.Contains(t, html, `<h1 class="heading heading--page">Settings</h1>`)
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
	assert.Contains(t, html, `data-action="edit-token" data-token-id="17" data-token-description="CI release pipeline" data-token-scopes="devices:read-update"`)
	assert.Contains(t, html, `id="tokenEditModal" aria-labelledby="token-edit-modal-title" aria-describedby="tokenEditStatus"`)
	assert.Contains(t, html, `id="edit-scope-devices:read" name="editScopes" value="devices:read"`)
	assert.Contains(t, html, `id="edit-scope-updates:read" name="editScopes" value="updates:read"`)
	assert.Contains(t, html, `id="tokenModal"`)
	assert.Contains(t, html, `id="scope-devices:read" name="scopes" value="devices:read"`)
	assert.Contains(t, html, `id="description" name="description" maxlength="80" autocomplete="off" required`)
	assert.Contains(t, html, `id="expires" name="expires" autocomplete="off" required`)
	assert.Contains(t, html, `id="revokeModal"`)
	assert.Contains(t, html, `<dialog class="upload-dialog" id="passwordModal"`)
	assert.Contains(t, html, `<button class="dlg-btn dlg-btn-cancel" type="button" data-action="close-password">Cancel</button>`)
	assert.Contains(t, html, `<button class="dlg-btn dlg-btn-upload" type="submit" form="passwordForm">Change password</button>`)
	assert.NotContains(t, html, `<header class="settings-ledger__dialog-head">`, "password dialog must use the shared .dlg-header")
	assert.Contains(t, html, `id="passwordError" role="alert" aria-live="assertive" tabindex="-1" style="display: none;"`)
	assert.Contains(t, html, `/users/operator/tokens`)
	assert.Contains(t, html, `submitPasswordChange(currentPassword, newPassword, confirmPassword, 'passwordError', function()`)
	assert.Contains(t, html, `<button class="settings-ledger__button" type="button" data-action="change-password">Change password</button>
          <p class="settings-ledger__account-status" id="passwordStatus" role="status" aria-live="polite"></p>`, "password confirmation must sit next to the button, which is styled as a button")
	assert.Contains(t, html, `document.getElementById('passwordStatus').textContent = '';
            setPasswordError(document.getElementById('passwordError'), '', false);
            document.getElementById('passwordForm').reset();
            passwordModal.showModal();`, "reopening the password dialog must clear the stale error")
	assert.Contains(t, html, `document.getElementById('passwordModal').close();
          document.getElementById('passwordStatus').textContent = 'Password changed.';
        });`, "a successful change must be announced after the dialog closes")
	assert.Contains(t, html, `class="sr-only" id="settingsStatus" role="status" aria-live="polite"`, "endpoint copy status stays screen-reader only")
	assert.Contains(t, html, `passwordModal.addEventListener('cancel', function(event) {
            if (passwordPending) {
              event.preventDefault();`, "Escape must not dismiss the dialog while a password request is pending")
	assert.Contains(t, html, `passwordModal.addEventListener('close', function() {
            if (passwordPending && !passwordModal.open) {
              passwordModal.showModal();
            }`, "a non-cancelable Escape must not leave a pending request in a closed dialog")
	assert.Contains(t, html, `passwordPending = false;
          document.getElementById('passwordModal').close();`, "success must clear the pending flag before closing")
	assert.Contains(t, html, `if (tokenCreating) {
          return;
        }`, "a second create click must not send another POST")
	assert.Contains(t, html, `tokenCreating = true;
        setTokenBusy('button[form="tokenForm"]', true);
        let created = false;
        fetch('/users/operator/tokens', {`, "create must lock before the request starts")
	assert.Contains(t, html, `tokenCreating = false;
          setTokenBusy('button[form="tokenForm"]', false);`, "create must unlock when the request settles")
	assert.Contains(t, html, `if (created) {
            document.getElementById('tokenForm').reset();
          }`, "failed creates must keep the user's input")
	assert.NotContains(t, html, `document.getElementById('description').value = '';`)
	assert.Contains(t, html, `if (tokenUpdating) {
          return;
        }`, "a second update click must not send another PATCH")
	assert.Contains(t, html, `tokenUpdating = true;
        setTokenBusy('button[form="tokenEditForm"]', true);
        fetch(`)
	assert.Contains(t, html, `tokenUpdating = false;
          setTokenBusy('button[form="tokenEditForm"]', false);`)
	assert.Contains(t, html, `if (tokenRevoking) {
          return;
        }`, "a second revoke click must not send another DELETE")
	assert.Contains(t, html, `tokenRevoking = true;
        setTokenBusy('[data-action="confirm-revoke"]', true);
        fetch(`)
	assert.Contains(t, html, `tokenRevoking = false;
          setTokenBusy('[data-action="confirm-revoke"]', false);`)
	assert.NotContains(t, html, `alert(`)
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

	assert.NotContains(t, html, `<button class="settings-ledger__button" type="button" data-action="change-password">`)
	assert.NotContains(t, html, `id="passwordStatus"`)
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
		`.settings-ledger__section-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 20px;
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
		`.settings-ledger__token-table th:last-child,
.settings-ledger__token-table td:last-child {
    width: 6.5rem;
    text-align: right;
    white-space: nowrap;
}`,
		`:root[data-theme="dark"] .settings-ledger__button--primary {
    border-color: var(--brand-accent);
    background: var(--brand-accent);
    color: var(--brand-primary);
}`,
		"#tokenCreatedModal,\n#revokeModal,\n#passwordModal {\n    font-size: 1rem;\n}",
		"    #revokeModal .dlg-close,\n    #revokeModal .dlg-btn,\n    #passwordModal .dlg-close,\n    #passwordModal .dlg-btn,\n    #tokenCreatedModal .btn-copy {\n        min-height: 44px;\n    }",
		"    #revokeModal .dlg-close,\n    #passwordModal .dlg-close {\n        min-width: 44px;\n    }",
		"@media (max-width: 620px)",
		"@media (pointer: coarse)",
		"@media (prefers-reduced-motion: reduce)",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("style.css does not contain %q", want)
		}
	}
	assert.NotContains(t, css, ".settings-ledger > h1 {")
	assert.NotContains(t, css, ".settings-ledger__dialog {")
	assert.NotContains(t, css, ".settings-ledger__dialog-close")
	assert.NotContains(t, css, ".settings-ledger__dialog-head")
	assert.NotContains(t, css, ".settings-ledger__dialog-body")
	assert.NotContains(t, css, ".settings-ledger__dialog-foot")
	assert.NotContains(t, css, ".settings-ledger__dialog >")
}

func TestSettingsTokenUpdate(t *testing.T) {
	tmpdir := t.TempDir()
	db, err := storage.NewDb(filepath.Join(tmpdir, "sql.db"))
	require.NoError(t, err)
	fs, err := storage.NewFs(tmpdir)
	require.NoError(t, err)
	require.NoError(t, fs.Auth.InitHmacSecret())
	userStorage, err := users.NewStorage(db, fs, &storage.AuthConfig{})
	require.NoError(t, err)
	u := users.User{Username: "operator", Password: "hash", AllowedScopes: users.ScopeDevicesRU | users.ScopeUpdatesR}
	require.NoError(t, userStorage.Create(&u))
	user, err := userStorage.Get("operator")
	require.NoError(t, err)
	tok, err := user.GenerateToken("before", time.Now().Add(time.Hour).Unix(), users.ScopeDevicesR)
	require.NoError(t, err)

	update := func(t *testing.T, tokenID, body string) int {
		t.Helper()
		e := echo.New()
		req := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		ctx := context.CtxWithLog(req.Context(), slog.New(slog.DiscardHandler))
		req = req.WithContext(CtxWithSession(ctx, &auth.Session{User: user}))
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("username", "tokenID")
		c.SetParamValues("operator", tokenID)
		if err := (handlers{}).settingsTokenUpdate(c); err != nil {
			var httpErr *echo.HTTPError
			require.ErrorAs(t, err, &httpErr)
			return httpErr.Code
		}
		return rec.Code
	}
	tokenID := strconv.FormatInt(tok.PublicID, 10)

	code := update(t, tokenID, `{"description":" after ","scopes":["devices:read-update","updates:read"]}`)
	require.Equal(t, http.StatusNoContent, code)
	tokens, err := user.ListTokens()
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	assert.Equal(t, "after", tokens[0].Description)
	assert.Equal(t, users.ScopeDevicesRU|users.ScopeUpdatesR, tokens[0].Scopes)

	for name, tc := range map[string]struct{ id, body string }{
		"bad id":          {"abc", `{"description":"x","scopes":["devices:read"]}`},
		"no description":  {tokenID, `{"description":"  ","scopes":["devices:read"]}`},
		"no scopes":       {tokenID, `{"description":"x","scopes":[]}`},
		"unknown scope":   {tokenID, `{"description":"x","scopes":["bogus"]}`},
		"exceeds allowed": {tokenID, `{"description":"x","scopes":["users:create"]}`},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, http.StatusBadRequest, update(t, tc.id, tc.body))
		})
	}
}
