// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"

	"github.com/foundriesio/update-server/auth"
	"github.com/foundriesio/update-server/clock"
	"github.com/foundriesio/update-server/server/ui/api"
	"github.com/foundriesio/update-server/server/ui/i18n"
	webtemplates "github.com/foundriesio/update-server/server/ui/web/templates"
	"github.com/foundriesio/update-server/storage/users"
)

// English msgids the Italian devices list must not show.
var devicesListEnglishLeaks = []string{
	"Registered devices",
	"Target",
	"Labels",
	"Group / Tag",
	"Last seen",
	"Actions",
	"Name",
	"Created",
	"Delete",
	"Delete device",
	"Cancel",
	"Close dialog",
	"No labels",
	"Devices pagination",
	"First page",
	"Previous page",
	"Next page",
	"Last page",
	"Sorted ascending, click for descending",
	"Sorted descending, click for ascending",
	"I understand this action cannot be undone",
	"Removing this device will disconnect it from all update targets.",
	"Historical data is not preserved.",
}

func devicesListItem(labels map[string]string) api.DeviceListItem {
	return api.DeviceListItem{Uuid: "00000000-0000-0000-0000-000000000001", Labels: labels}
}

func renderDevicesListCtx(t *testing.T, ctx devicesListCtx) string {
	t.Helper()

	var buf bytes.Buffer
	require.NoError(t, webtemplates.Templates.ExecuteTemplate(&buf, "devices_list.html", ctx), "ExecuteTemplate(devices_list.html, %s)", ctx.L.Tag())

	return buf.String()
}

func renderDevicesListHTML(t *testing.T, tag language.Tag, devices []api.DeviceListItem) string {
	t.Helper()

	return renderDevicesListCtx(t, devicesListCtx{
		baseCtx:    baseCtx{L: i18n.For(tag), BrandName: "Update Server"},
		Devices:    devices,
		CanDelete:  true,
		Page:       1,
		TotalPages: 1,
		Sort:       "created-at-desc",
	})
}

func assertNoEnglishLeaks(t *testing.T, got string) {
	t.Helper()

	// Msgids whose Italian text is identical to the English, so a leak cannot be told apart.
	sameInItalian := map[string]bool{"Target": true}
	it := i18n.For(language.Italian)
	for _, msgid := range devicesListEnglishLeaks {
		if sameInItalian[msgid] {
			continue
		}
		require.NotEqual(t, msgid, it.T(msgid), "missing it translation for %q", msgid)
		// Whole-token match: a bare substring would hit JS identifiers such as deleteName.
		leak := regexp.MustCompile(`(^|[>"\s])` + regexp.QuoteMeta(msgid) + `($|[<"\s])`)
		assert.NotRegexp(t, leak, got, "English leak %q in Italian output", msgid)
	}
}

func TestDevicesListRendersEnglish(t *testing.T) {
	got := renderDevicesListHTML(t, language.English, []api.DeviceListItem{devicesListItem(map[string]string{"name": "alpha"})})

	assert.Contains(t, got, `<html lang="en" dir="ltr">`)
	assert.Contains(t, got, "Registered devices")
	assert.Contains(t, got, `title="Sort by Name"`)
	assert.Contains(t, got, `aria-label="Delete device alpha"`)
}

func TestDevicesListRendersItalian(t *testing.T) {
	tag := i18n.Resolve("", "it")
	got := renderDevicesListHTML(t, tag, []api.DeviceListItem{devicesListItem(map[string]string{"name": "alpha"})})

	assert.Contains(t, got, `<html lang="it" dir="ltr">`)
	assert.Contains(t, got, "Dispositivi registrati")
	assert.Contains(t, got, `title="Ordina per Nome"`)
	assert.Contains(t, got, `aria-label="Elimina dispositivo alpha"`)
	assert.NotContains(t, got, "%!", "formatting error in output")
	assert.NotContains(t, got, "%[1]", "unformatted placeholder in output")

	assertNoEnglishLeaks(t, got)
}

func TestDevicesListPaginationAndSortStates(t *testing.T) {
	devices := []api.DeviceListItem{devicesListItem(map[string]string{"name": "alpha"})}
	tests := []struct {
		name string
		sort string
		en   []string
		it   []string
	}{
		{
			name: "name-asc",
			sort: "name-asc",
			en: []string{
				`<span class="pagination-summary">Page 2 of 3</span>`,
				`aria-label="Previous page"`,
				`aria-label="Page 2"`,
				`title="Sort by Created"`,
				`title="Sorted ascending, click for descending"`,
			},
			it: []string{
				`<span class="pagination-summary">Pagina 2 di 3</span>`,
				`aria-label="Pagina precedente"`,
				`aria-label="Pagina 2"`,
				`title="Ordina per Creato"`,
				`title="Ordine crescente, clicca per decrescente"`,
			},
		},
		{
			name: "name-desc",
			sort: "name-desc",
			en:   []string{`title="Sorted descending, click for ascending"`},
			it:   []string{`title="Ordine decrescente, clicca per crescente"`},
		},
		{
			name: "created-at-asc",
			sort: "created-at-asc",
			en:   []string{`title="Sort by Name"`, `title="Sorted ascending, click for descending"`},
			it:   []string{`title="Ordina per Nome"`, `title="Ordine crescente, clicca per decrescente"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := func(tag language.Tag) devicesListCtx {
				return devicesListCtx{
					baseCtx:    baseCtx{L: i18n.For(tag), BrandName: "Update Server"},
					Devices:    devices,
					CanDelete:  true,
					Page:       2,
					TotalPages: 3,
					HasPrev:    true,
					HasNext:    true,
					Sort:       tt.sort,
				}
			}

			gotEn := renderDevicesListCtx(t, ctx(language.English))
			for _, want := range tt.en {
				assert.Contains(t, gotEn, want)
			}

			gotIt := renderDevicesListCtx(t, ctx(i18n.Resolve("", "it")))
			for _, want := range tt.it {
				assert.Contains(t, gotIt, want)
			}
			assertNoEnglishLeaks(t, gotIt)
		})
	}
}

func TestDevicesListSortGlyphsHiddenFromAT(t *testing.T) {
	got := renderDevicesListHTML(t, language.English, []api.DeviceListItem{devicesListItem(map[string]string{"name": "alpha"})})

	assert.Contains(t, got, `<span class="sort-active" aria-hidden="true">▼</span>`)
	assert.Contains(t, got, `<span class="sort-idle" aria-hidden="true">⇅</span>`)
	assert.NotContains(t, got, `<span class="sort-active">`)
	assert.NotContains(t, got, `<span class="sort-idle">`)
}

func TestDevicesListDeleteErrorFormat(t *testing.T) {
	devices := []api.DeviceListItem{devicesListItem(map[string]string{"name": "alpha"})}

	assert.Contains(t, renderDevicesListHTML(t, language.English, devices),
		`data-error-format="Error deleting device: {error}"`)
	assert.Contains(t, renderDevicesListHTML(t, i18n.Resolve("", "it"), devices),
		`data-error-format="Errore durante l&#39;eliminazione del dispositivo: {error}"`)
}

func TestDevicesListHandlerLocalizesTitle(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	t.Cleanup(upstream.Close)

	h := handlers{templates: webtemplates.Templates, pages: NewPageBuilder(Branding{Title: "Update Server"})}
	tests := []struct {
		accept string
		h1     string
	}{
		{accept: "it", h1: `<h1 class="heading heading--page">Dispositivi</h1>`},
		{accept: "en", h1: `<h1 class="heading heading--page">Devices</h1>`},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/devices", nil)
		req.Header.Set("Accept-Language", tt.accept)
		session := &auth.Session{
			BaseUrl: upstream.URL,
			Client:  upstream.Client(),
			User:    &users.User{AllowedScopes: users.ScopeDevicesR},
		}
		req = req.WithContext(CtxWithSession(req.Context(), session)) //nolint:unqueryvet // HTTP context setup does not start a transaction.
		rec := httptest.NewRecorder()

		require.NoError(t, h.devicesList(echo.New().NewContext(req, rec)))
		assert.Contains(t, rec.Body.String(), tt.h1, "Accept-Language %s", tt.accept)
	}
}

func TestDevicesListLabelCountPlurals(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		en     string
		it     string
	}{
		{
			name:   "none",
			labels: map[string]string{"name": "alpha"},
			en:     `aria-label="No labels"`,
			it:     `aria-label="Nessuna etichetta"`,
		},
		{
			name:   "one",
			labels: map[string]string{"name": "alpha", "env": "prod", "site": "dallas"},
			en:     `aria-label="Show 1 more label"`,
			it:     `aria-label="Mostra 1 altra etichetta"`,
		},
		{
			name:   "many",
			labels: map[string]string{"name": "alpha", "a": "1", "b": "2", "c": "3", "d": "4"},
			en:     `aria-label="Show 3 more labels"`,
			it:     `aria-label="Mostra 3 altre etichette"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			devices := []api.DeviceListItem{devicesListItem(tt.labels)}

			assert.Contains(t, renderDevicesListHTML(t, language.English, devices), tt.en)
			assert.Contains(t, renderDevicesListHTML(t, i18n.Resolve("", "it"), devices), tt.it)
		})
	}
}

func TestDevicesListEscapesDeviceName(t *testing.T) {
	name := `<script>alert(1)</script>`
	devices := []api.DeviceListItem{devicesListItem(map[string]string{"name": name})}

	for _, tag := range []language.Tag{language.English, i18n.Resolve("", "it")} {
		got := renderDevicesListHTML(t, tag, devices)

		assert.NotContains(t, got, name, "unescaped device name in %s output", tag)
		assert.Contains(t, got, "&lt;script&gt;alert(1)&lt;/script&gt;", "escaped device name in %s output", tag)
	}
}

func TestDevicesListEscapesHostileDeviceName(t *testing.T) {
	name := `');alert(1);//<script>`
	devices := []api.DeviceListItem{devicesListItem(map[string]string{"name": name})}

	for _, tag := range []language.Tag{language.English, i18n.Resolve("", "it")} {
		got := renderDevicesListHTML(t, tag, devices)

		assert.NotContains(t, got, `');alert(1)`, "quote breakout in %s output", tag)
		assert.NotContains(t, got, `//<script>`, "raw script tag in %s output", tag)
		assert.Contains(t, got, `&#39;);alert(1);//&lt;script&gt;`, "escaped text node in %s output", tag)
		assert.Contains(t, got, `onclick="showDeleteDialog('00000000-0000-0000-0000-000000000001', '\u0027);alert(1);\/\/\u003cscript\u003e')"`, "escaped onclick in %s output", tag)
	}
}

func TestDevicesListLocalizesCreatedDate(t *testing.T) {
	devices := []api.DeviceListItem{{Uuid: "dev", CreatedAt: 1_780_000_000}}

	gotIt := renderDevicesListHTML(t, language.Italian, devices)

	assert.Contains(t, gotIt, `<td class="cell-ts">28 mag 2026</td>`)

	gotEn := renderDevicesListHTML(t, language.English, devices)

	assert.Contains(t, gotEn, `<td class="cell-ts">May 28, 2026</td>`)
}

func devicesListTimestamps(t *testing.T, tag language.Tag) string {
	t.Helper()

	now := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	old := clock.Now
	clock.Now = func() time.Time { return now }
	t.Cleanup(func() { clock.Now = old })

	ago := func(d time.Duration) int64 { return now.Add(-d).Unix() }
	devices := []api.DeviceListItem{
		{Uuid: "never", CreatedAt: 1_780_000_000},
		{Uuid: "five-minutes", CreatedAt: 1_780_000_000, LastSeen: ago(5 * time.Minute)},
		{Uuid: "one-hour", CreatedAt: 1_780_000_000, LastSeen: ago(time.Hour)},
		{Uuid: "three-days", CreatedAt: 1_780_000_000, LastSeen: ago(72 * time.Hour)},
		{Uuid: "old", CreatedAt: 1_780_000_000, LastSeen: ago(30 * 24 * time.Hour)},
	}

	return renderDevicesListHTML(t, tag, devices)
}

func TestDevicesListLocalizesRelativeTimes(t *testing.T) {
	gotIt := devicesListTimestamps(t, language.Italian)

	assert.Contains(t, gotIt, `<td class="cell-ts">Mai</td>`)
	assert.Contains(t, gotIt, ">5 minuti fa</time>")
	assert.Contains(t, gotIt, ">1 ora fa</time>")
	assert.Contains(t, gotIt, ">3 giorni fa</time>")
	assert.Contains(t, gotIt, ">28 set 2026</time>")
	assert.NotContains(t, gotIt, " ago<")
	assert.NotContains(t, gotIt, "Never")

	gotEn := devicesListTimestamps(t, language.English)

	assert.Contains(t, gotEn, `<td class="cell-ts">Never</td>`)
	assert.Contains(t, gotEn, ">5 minutes ago</time>")
	assert.Contains(t, gotEn, ">1 hour ago</time>")
	assert.Contains(t, gotEn, ">3 days ago</time>")
	assert.Contains(t, gotEn, ">Sep 28, 2026</time>")
}
