// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/foundriesio/update-server/auth"
	"github.com/foundriesio/update-server/context"
	"github.com/foundriesio/update-server/server/ui/i18n"
	webtemplates "github.com/foundriesio/update-server/server/ui/web/templates"
	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/users"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"
)

type settingsTemplateCtx struct {
	baseCtx
	Tokens            []users.Token
	ScopesList        []string
	GrantableScopes   []string
	ScopeGroups       []settingsScopeTemplateGroup
	LocalAuth         bool
	DeviceApiUrl      string
	Oauth2ApiUrl      string
	Languages         []i18n.Language
	StoredLocale      string
	EffectiveLanguage string
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
			L:     i18n.For(language.English),
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
          document.getElementById('passwordStatus').textContent = messages.passwordChanged;
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
		baseCtx:    baseCtx{Title: "Settings", L: i18n.For(language.English), User: &users.User{Username: "operator"}},
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
	}, i18n.For(language.English)))
}

func TestSettingsTemplateHidesLocalPasswordDialogForExternalAuth(t *testing.T) {
	html := renderSettingsHTML(t, settingsTemplateCtx{
		baseCtx: baseCtx{
			Title: "Settings",
			L:     i18n.For(language.English),
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

// hostileMsgid would close the script tag and inject markup if it were written raw.
const hostileMsgid = `</script><script>alert(1)</script>"<b>`

type hostilePrinter struct{ i18n.Printer }

func (hostilePrinter) T(string, ...any) string { return hostileMsgid }

func (hostilePrinter) N(string, string, int, ...any) string { return hostileMsgid }

func settingsFullCtx(L i18n.Printer) settingsTemplateCtx {
	return settingsTemplateCtx{
		baseCtx: baseCtx{
			Title: L.T("Settings"),
			L:     L,
			User: &users.User{
				Username:      "operator",
				Email:         "operator@example.test",
				AllowedScopes: users.ScopeDevicesRU | users.ScopeUpdatesR,
			},
		},
		Tokens: []users.Token{
			{PublicID: 17, CreatedAt: 1_780_000_000, ExpiresAt: 1_780_086_400, Description: "CI release pipeline", Scopes: users.ScopeDevicesRU},
			{PublicID: 18, CreatedAt: 1_700_000_000, ExpiresAt: 1_000_000, Description: "Old token", Scopes: users.ScopeDevicesR},
		},
		ScopesList: []string{"devices:read-update", "updates:read"},
		ScopeGroups: []settingsScopeTemplateGroup{
			{Resource: "devices", Operations: "Read or update", ScopeNames: []string{"devices:read-update"}},
			{Resource: "updates", Operations: "Read", ScopeNames: []string{"updates:read"}},
		},
		LocalAuth:         true,
		DeviceApiUrl:      "https://updates.example.test/v1/devices",
		Oauth2ApiUrl:      "https://updates.example.test/oauth2",
		Languages:         i18n.Languages(),
		EffectiveLanguage: "English",
	}
}

func settingsEmptyCtx(L i18n.Printer) settingsTemplateCtx {
	return settingsTemplateCtx{
		baseCtx:      baseCtx{Title: L.T("Settings"), L: L, User: &users.User{Username: "operator"}},
		DeviceApiUrl: "https://updates.example.test/v1/devices",
		Oauth2ApiUrl: "https://updates.example.test/oauth2",
		Languages:    i18n.Languages(),
		StoredLocale: "it",
	}
}

// English text shown in both settings states; each msgid must be in the English render.
var settingsCommonEnglish = []string{
	"Settings", "Account", "Audit log →", "Username", "Email", "ID", "Language", "Display language",
	"Automatic (browser language)", "Save language", "Allowed scopes", "Resource", "Operations",
	"Scope names", "Service endpoints", "For fio-device-register", "Device API", "OAuth2 API", "Copy",
	"Copy Device API endpoint", "Copy OAuth2 API endpoint",
	"API tokens", "New token", "Description", "Expires on", "Scopes", "Revoke token",
	"Create API token", "Close dialog", "Create token", "Select one or more permissions for this token.",
	"Token created", "This is the only time this token is shown. Copy and store it securely.",
	"API token", "Copy API token", "Close", "Revoke API token", "Revoking this token cannot be undone.",
	"Cancel", "Edit API token", "OK",
}

var settingsFullOnlyEnglish = []string{
	"Change password", "Current password", "New password", "Confirm new password",
	"Passwords do not match.", "Expired", "Created", "Expires", "Actions", "Edit token",
}

var settingsEmptyOnlyEnglish = []string{
	"Not configured", "No scopes assigned", "No API tokens. Use the New token button to create one.",
}

// Msgids whose Italian text equals the English; TestSettingsSameInItalianMsgidsAreWrapped covers them.
var settingsSameInItalian = []string{"Account", "Email", "ID", "OK"}

// assertSettingsNoEnglish fails on any msgid whose English text survives in got.
func assertSettingsNoEnglish(t *testing.T, got string, msgids []string) {
	t.Helper()

	it := i18n.For(language.Italian)
	for _, msgid := range msgids {
		if slices.Contains(settingsSameInItalian, msgid) {
			continue
		}
		require.NotEqual(t, msgid, it.T(msgid), "missing it translation for %q", msgid)
		// Whole-token match, as in the devices list leak check.
		leak := regexp.MustCompile(`(^|[>"\s])` + regexp.QuoteMeta(msgid) + `($|[<"\s])`)
		assert.NotRegexp(t, leak, got, "English leak %q in Italian output", msgid)
	}
}

func TestSettingsTemplateItalianStatesHaveNoEnglish(t *testing.T) {
	tests := []struct {
		name   string
		ctx    func(i18n.Printer) settingsTemplateCtx
		msgids []string
	}{
		{
			name:   "local auth with tokens and scopes",
			ctx:    settingsFullCtx,
			msgids: slices.Concat(settingsCommonEnglish, settingsFullOnlyEnglish),
		},
		{
			name:   "external auth without tokens or scopes",
			ctx:    settingsEmptyCtx,
			msgids: slices.Concat(settingsCommonEnglish, settingsEmptyOnlyEnglish),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEn := renderSettingsHTML(t, tt.ctx(i18n.For(language.English)))
			for _, msgid := range tt.msgids {
				require.Contains(t, gotEn, msgid, "English render lacks %q, so the leak check would pass vacuously", msgid)
			}

			gotIt := renderSettingsHTML(t, tt.ctx(i18n.For(language.Italian)))
			assert.NotContains(t, gotIt, "%!", "formatting error in output")
			assertSettingsNoEnglish(t, gotIt, tt.msgids)
			// Revoke token takes a format argument, so the msgid regex cannot match its rendered form.
			assert.NotContains(t, gotIt, `aria-label="Revoke token CI release pipeline"`)
			assert.NotContains(t, gotIt, `aria-label="Revoke token Old token"`)
			assert.NotContains(t, gotIt, `aria-label="Edit token CI release pipeline"`)
		})
	}
}

func TestSettingsTemplateItalianStatesRenderEffectiveLanguage(t *testing.T) {
	got := renderSettingsHTML(t, settingsFullCtx(i18n.For(language.Italian)))
	assert.Contains(t, got, "Attualmente in uso: English")
	assert.NotContains(t, got, "Currently using")

	got = renderSettingsHTML(t, settingsEmptyCtx(i18n.For(language.Italian)))
	assert.NotContains(t, got, "Attualmente in uso")
}

func TestSettingsScriptMessagesAreDataAttributes(t *testing.T) {
	tests := []struct {
		name string
		tag  language.Tag
		want []string
	}{
		{
			name: "en",
			tag:  language.English,
			want: []string{
				`data-copy-unavailable="Copy is unavailable. Select the endpoint and copy it manually."`,
				`data-endpoint-copied="Endpoint copied."`,
				`data-token-fields-required="Enter a description and expiration date, then select at least one scope."`,
				`data-token-expiry-invalid="Expiration must be a valid future date."`,
				`data-token-create-failed="Unable to create token: {error}"`,
				`data-revoke-failed="Unable to revoke token: {error}"`,
				`data-token-copy-manual="Couldn’t copy automatically. Press Ctrl&#43;C or Command&#43;C, or touch and hold the token to copy."`,
				`data-token-copied="Token copied."`,
				`data-mismatch="New password and confirmation do not match."`,
				`data-failed-format="Failed to change password: {error}"`,
				`data-token-scopes-dropped="Saving will remove scopes you can no longer grant: {scopes}"`,
				`data-token-edit-required="Enter a description and select at least one scope."`,
				`data-token-update-failed="Unable to update token: {error}"`,
			},
		},
		{
			name: "it",
			tag:  language.Italian,
			want: []string{
				`data-copy-unavailable="La copia non è disponibile. Seleziona l&#39;endpoint e copialo manualmente."`,
				`data-endpoint-copied="Endpoint copiato."`,
				`data-token-fields-required="Inserisci una descrizione e una data di scadenza, poi seleziona almeno un ambito."`,
				`data-token-expiry-invalid="La scadenza deve essere una data futura valida."`,
				`data-token-create-failed="Creazione del token non riuscita: {error}"`,
				`data-revoke-failed="Revoca del token non riuscita: {error}"`,
				`data-token-copy-manual="Copia automatica non riuscita. Premi Ctrl&#43;C o Command&#43;C, oppure tieni premuto il token per copiarlo."`,
				`data-token-copied="Token copiato."`,
				`data-mismatch="La nuova password e la conferma non coincidono."`,
				`data-failed-format="Modifica della password non riuscita: {error}"`,
				`data-token-scopes-dropped="Il salvataggio rimuoverà gli ambiti che non puoi più concedere: {scopes}"`,
				`data-token-edit-required="Inserisci una descrizione e seleziona almeno un ambito."`,
				`data-token-update-failed="Aggiornamento del token non riuscito: {error}"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderSettingsHTML(t, settingsFullCtx(i18n.For(tt.tag)))
			for _, want := range tt.want {
				assert.Contains(t, got, want)
			}
		})
	}
}

var (
	settingsPageTagRE     = regexp.MustCompile(`<section class="settings-ledger device-page-inner" id="settingsPage"[^>]*>`)
	settingsPasswordTagRE = regexp.MustCompile(`<form id="passwordForm"[^>]*>`)
	settingsDataAttrRE    = regexp.MustCompile(`\s(data-[a-z-]+)="`)
)

// settingsMessageAttrNames lists the data-* attributes on #settingsPage and the password form.
func settingsMessageAttrNames(html string) []string {
	var names []string
	for _, tagRE := range []*regexp.Regexp{settingsPageTagRE, settingsPasswordTagRE} {
		for _, m := range settingsDataAttrRE.FindAllStringSubmatch(tagRE.FindString(html), -1) {
			names = append(names, m[1])
		}
	}
	return names
}

// msgidHostilePrinter returns the hostile string for one msgid and the real translation for the rest.
type msgidHostilePrinter struct {
	i18n.Printer
	msgid string
}

func (p msgidHostilePrinter) T(msgid string, args ...any) string {
	if msgid == p.msgid {
		return hostileMsgid
	}
	return p.Printer.T(msgid, args...)
}

func TestSettingsTemplateEscapesHostileTranslation(t *testing.T) {
	en := renderSettingsHTML(t, settingsFullCtx(i18n.For(language.English)))
	got := renderSettingsHTML(t, settingsFullCtx(hostilePrinter{i18n.For(language.English)}))

	// The hostile string must reach the page only as escaped text or attribute values.
	assert.Equal(t, strings.Count(en, "</script>"), strings.Count(got, "</script>"), "catalog string closed a script tag")
	assert.NotContains(t, got, `<script>alert(1)</script>`)
	assert.NotContains(t, got, `"<b>`)

	const escaped = `&lt;/script&gt;&lt;script&gt;alert(1)&lt;/script&gt;&#34;&lt;b&gt;`
	names := settingsMessageAttrNames(en)
	require.Contains(t, names, "data-copy-unavailable", "settings page attributes not found")
	require.Contains(t, names, "data-failed-format", "password form attributes not found")
	for _, name := range names {
		assert.Contains(t, got, name+`="`+escaped+`"`)
	}
	// Revoke button: aria-label and title both come from catalog strings.
	assert.Contains(t, got, `data-token-id="17" aria-label="`+escaped+`" title="`+escaped+`"`)
}

// Msgids identical in Italian pass the leak check, so prove each one is wrapped in L.T.
func TestSettingsSameInItalianMsgidsAreWrapped(t *testing.T) {
	const escaped = `&lt;/script&gt;&lt;script&gt;alert(1)&lt;/script&gt;&#34;&lt;b&gt;`
	for _, msgid := range settingsSameInItalian {
		t.Run(msgid, func(t *testing.T) {
			got := renderSettingsHTML(t, settingsFullCtx(msgidHostilePrinter{i18n.For(language.Italian), msgid}))
			assert.Contains(t, got, escaped, "%q is not rendered through L.T", msgid)
		})
	}
}

func TestSettingsScopeSentenceCaseKeepsMultiByteFirstRune(t *testing.T) {
	assert.Equal(t, "Élément", settingsScopeSentenceCase("élément"))
}

func TestGroupSettingsScopesTranslatesOperations(t *testing.T) {
	got := groupSettingsScopes([]string{
		"devices:read-update",
		"devices:create",
		"devices:delete",
		"updates:delete",
		"updates:read-update",
	}, i18n.For(language.Italian))

	assert.Equal(t, []settingsScopeGroup{
		{
			Resource:   "devices",
			Operations: "Creazione, eliminazione, lettura o aggiornamento",
			ScopeNames: []string{"devices:read-update", "devices:create", "devices:delete"},
		},
		{
			Resource:   "updates",
			Operations: "Eliminazione, lettura o aggiornamento",
			ScopeNames: []string{"updates:delete", "updates:read-update"},
		},
	}, got)
}

func TestSettingsPageLocalizesTitleAndOperations(t *testing.T) {
	strg := newLocaleTestStorage(t)
	e := newLocaleTestServer(t, strg, newLocaleTestUser(t, strg, "it"))

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `<h1 class="heading heading--page">Impostazioni</h1>`)
	assert.Contains(t, rec.Body.String(), `<td>Lettura</td>`)
}

func TestSettingsTemplateLocalizesTokenDates(t *testing.T) {
	gotIt := renderSettingsHTML(t, settingsFullCtx(i18n.For(language.Italian)))

	assert.Contains(t, gotIt, `<time datetime="2026-05-28T20:26:40Z">28 mag 2026</time>`)
	assert.Contains(t, gotIt, `<time datetime="2026-05-29T20:26:40Z">29 mag 2026</time>`)
	assert.Contains(t, gotIt, `<time datetime="2023-11-14T22:13:20Z">14 nov 2023</time>`)

	gotEn := renderSettingsHTML(t, settingsFullCtx(i18n.For(language.English)))

	assert.Contains(t, gotEn, `<time datetime="2026-05-28T20:26:40Z">May 28, 2026</time>`)
	assert.Contains(t, gotEn, `<time datetime="2023-11-14T22:13:20Z">Nov 14, 2023</time>`)
}
