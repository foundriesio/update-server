// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"bytes"
	"html/template"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/foundriesio/update-server/server/ui/api"
	"github.com/foundriesio/update-server/server/ui/web/templates"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

// updateDetailCtx mirrors the anonymous context struct updatesGet builds for
// update.html, so tests can render the template with representative data.
type updateDetailCtx struct {
	baseCtx
	Tag           string
	Name          string
	Summary       api.UpdateSummary
	Rollouts      []rolloutSummary
	LatestRollout *rolloutSummary
	Groups        []string
	Tuf           api.UpdateTufResp
	TufJson       string
	LatestTarget  *latestTarget
	TufMetadata   []tufMetadataItem
	TufError      string
}

type updateRolloutDetailCtx struct {
	baseCtx
	Tag     string
	Name    string
	Rollout string
	Details api.Rollout
	Summary api.UpdateSummary
}

func renderUpdateHTML(t *testing.T, ctx updateDetailCtx) string {
	t.Helper()
	var buf bytes.Buffer
	if err := templates.Templates.ExecuteTemplate(&buf, "update.html", ctx); err != nil {
		t.Fatalf("ExecuteTemplate() error = %v", err)
	}
	return buf.String()
}

func renderUpdateRolloutHTML(t *testing.T, ctx updateRolloutDetailCtx) string {
	t.Helper()
	var buf bytes.Buffer
	if err := templates.Templates.ExecuteTemplate(&buf, "update_rollout.html", ctx); err != nil {
		t.Fatalf("ExecuteTemplate() error = %v", err)
	}
	return buf.String()
}

func renderUpdateTailHTML(t *testing.T, ctx tailStreamCtx) string {
	t.Helper()
	var buf bytes.Buffer
	if err := templates.Templates.ExecuteTemplate(&buf, "update_tail.html", ctx); err != nil {
		t.Fatalf("ExecuteTemplate() error = %v", err)
	}
	return buf.String()
}

func renderRolloutTailHTML(t *testing.T, ctx tailStreamCtx) string {
	t.Helper()
	var buf bytes.Buffer
	if err := templates.Templates.ExecuteTemplate(&buf, "rollout_tail.html", ctx); err != nil {
		t.Fatalf("ExecuteTemplate() error = %v", err)
	}
	return buf.String()
}

func TestUpdateTailTemplateRendersUpdateStream(t *testing.T) {
	html := renderUpdateTailHTML(t, tailStreamCtx{Name: "148", TailUrl: "/v1/updates/148/tail"})
	assert.Contains(t, html, `href="/updates/148">148</a>`)
	assert.Contains(t, html, `aria-current="page">Progress</span>`)
	assert.Contains(t, html, `<h1>148 Stream</h1>`)
	assert.Contains(t, html, `class="btn-ghost rollout-stream__back" href="/updates/148">Back to Update</a>`)
	assert.Contains(t, html, `<dt>Update</dt>`)
	assert.Contains(t, html, `class="rollout-stream__mono" translate="no">148</dd>`)
	assert.Contains(t, html, `Raw update records in arrival order.`)
	assert.NotContains(t, html, `<dt>Rollout</dt>`)
	assert.NotContains(t, html, `/rollouts/`)
}

func TestRolloutTailTemplateRendersRolloutStream(t *testing.T) {
	html := renderRolloutTailHTML(t, tailStreamCtx{
		Name:    "148",
		Rollout: "production",
		TailUrl: "/v1/updates/148/rollouts/production/tail",
	})
	assert.Contains(t, html, `href="/updates/148">148</a>`)
	assert.Contains(t, html, `href="/updates/148/rollouts/production">production</a>`)
	assert.Contains(t, html, `aria-current="page">Progress</span>`)
	assert.Contains(t, html, `<h1>production Stream</h1>`)
	assert.Contains(t, html, `class="btn-ghost rollout-stream__back" href="/updates/148/rollouts/production">Back to Rollout</a>`)
	assert.Contains(t, html, `<dt>Rollout</dt>`)
	assert.Contains(t, html, `Raw rollout records in arrival order.`)
}

func TestTailHandlersUseRouteSpecificTemplates(t *testing.T) {
	tailTemplates := template.Must(template.New("").Parse(`
		{{ define "update_tail.html" }}update:{{.Name}}:{{.Rollout}}:{{.TailUrl}}{{ end }}
		{{ define "rollout_tail.html" }}rollout:{{.Name}}:{{.Rollout}}:{{.TailUrl}}{{ end }}
	`))
	h := handlers{templates: tailTemplates, pages: NewPageBuilder(Branding{})}

	tests := []struct {
		name        string
		path        string
		paramNames  []string
		paramValues []string
		handler     echo.HandlerFunc
		want        string
	}{
		{
			name:        "update tail",
			path:        "/updates/148/tail",
			paramNames:  []string{"name"},
			paramValues: []string{"148"},
			handler:     h.updatesTail,
			want:        "update:148::/v1/updates/148/tail",
		},
		{
			name:        "rollout tail",
			path:        "/updates/148/rollouts/production/tail",
			paramNames:  []string{"name", "rollout"},
			paramValues: []string{"148", "production"},
			handler:     h.updatesRolloutTail,
			want:        "rollout:148:production:/v1/updates/148/rollouts/production/tail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			recorder := httptest.NewRecorder()
			ctx := e.NewContext(httptest.NewRequest(http.MethodGet, tt.path, nil), recorder)
			ctx.SetPath(tt.path)
			ctx.SetParamNames(tt.paramNames...)
			ctx.SetParamValues(tt.paramValues...)

			if err := tt.handler(ctx); err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			assert.Equal(t, tt.want, recorder.Body.String())
		})
	}
}

func assertDevicesDialogUsesModalFocus(t *testing.T, html string) {
	t.Helper()
	assert.Contains(t, html, `<dialog id="devicesModal"`)
	assert.Contains(t, html, `aria-labelledby="devices-dlg-title"`)
	assert.Contains(t, html, `id="devices-dlg-title" tabindex="-1"`)
	assert.Contains(t, html, `showDevicesModal(`)
	assert.Contains(t, html, `, this)"`)
	assert.NotContains(t, html, `return false;`)
	assert.Contains(t, html, `devicesDialog.showModal();`)
	assert.Contains(t, html, `devicesDialogTitle.focus();`)
	assert.Contains(t, html, `id="devices-dlg-close-btn"`)
	assert.Contains(t, html, `id="devices-dlg-footer-close-btn"`)
	assert.Contains(t, html, `devicesDialogClose.addEventListener('click'`)
	assert.Contains(t, html, `devicesDialogFooterClose.addEventListener('click'`)
	assert.Contains(t, html, `devicesDialog.addEventListener('close'`)
	assert.Contains(t, html, `devicesDialogTrigger.focus();`)
}

func TestUpdateTemplatePopulated(t *testing.T) {
	target := &latestTarget{
		Name:         "main/148",
		Version:      "148",
		Sha256:       "aaa111",
		Architecture: "amd64",
		HardwareIDs:  []string{"intel-corei7-64"},
		Tags:         []string{"main"},
		Apps: []releaseApplication{
			{Name: "nginx", URI: "registry/nginx@sha256:bbb"},
			{Name: "shellhttpd", URI: "registry/shellhttpd@sha256:aaa"},
		},
	}
	latest := rolloutSummary{Name: "r2", State: "Scheduled", DeviceCount: 3}
	ctx := updateDetailCtx{
		Name:    "148",
		Summary: api.UpdateSummary{Status: map[string]int{"in-sync": 2}},
		Rollouts: []rolloutSummary{
			{Name: "r1", State: "Preparing", DeviceCount: 1},
			latest,
		},
		LatestRollout: &latest,
		Groups:        []string{"factory"},
		TufJson:       "{}",
		LatestTarget:  target,
		TufMetadata: []tufMetadataItem{
			{Name: "Root", Expires: "2031-01-01T00:00:00Z"},
			{Name: "Timestamp", Expires: "2031-01-01T00:00:00Z"},
			{Name: "Snapshot", Expires: "2031-01-01T00:00:00Z"},
			{Name: "Targets", Expires: "2031-01-01T00:00:00Z"},
		},
	}

	html := renderUpdateHTML(t, ctx)

	assert.Contains(t, html, `class="update-detail"`)
	assert.Contains(t, html, `href="/updates/148/tail"`)
	assert.Contains(t, html, `onclick="rolloutModal.showModal()"`)
	assert.Contains(t, html, `id="rolloutModal"`)
	assert.Contains(t, html, `aria-live="polite"`)
	assert.Contains(t, html, `aria-label="Copy nginx image URI"`)
	assert.Contains(t, html, "Release manifest")
	assert.NotContains(t, html, "devices succeeded")
	assert.Contains(t, html, ">Valid<")

	nginxIdx := strings.Index(html, "nginx")
	shellIdx := strings.Index(html, "shellhttpd")
	assert.Greater(t, shellIdx, nginxIdx, "nginx should render before shellhttpd")
}

func TestUpdateTemplateTufError(t *testing.T) {
	ctx := updateDetailCtx{
		Name:        "148",
		TufError:    "Unable to look up the TUF metadata",
		TufMetadata: releaseTufMetadata(api.UpdateTufResp{}),
	}
	html := renderUpdateHTML(t, ctx)
	assert.Contains(t, html, "Unable to look up the TUF metadata")
	assert.NotContains(t, html, ">Valid<")
}

func TestUpdateTemplateNoLatestTarget(t *testing.T) {
	ctx := updateDetailCtx{Name: "148", TufMetadata: releaseTufMetadata(api.UpdateTufResp{})}
	html := renderUpdateHTML(t, ctx)
	assert.Contains(t, html, "No release information available")
}

func TestUpdateTemplateNoRollouts(t *testing.T) {
	ctx := updateDetailCtx{Name: "148", TufMetadata: releaseTufMetadata(api.UpdateTufResp{})}
	html := renderUpdateHTML(t, ctx)
	assert.Contains(t, html, "No rollouts yet.")
}

func TestUpdateTemplateUsesNativeNavigationAndDialogControls(t *testing.T) {
	ctx := updateDetailCtx{
		Name:        "148",
		Summary:     api.UpdateSummary{Status: map[string]int{"in-sync": 2}},
		TufMetadata: releaseTufMetadata(api.UpdateTufResp{}),
	}
	html := renderUpdateHTML(t, ctx)

	assert.Contains(t, html, `class="btn-ghost update-detail__progress" href="/updates/148/tail"`)
	assert.NotContains(t, html, `onclick='location.href="/updates/148/tail";'`)
	assert.Contains(t, html, `type="button" class="history-entry__target history-entry__action" onclick="showDevicesModal('in-sync', this)"`)
	assert.NotContains(t, html, `href="#" class="history-entry__target"`)
}

func TestUpdateTemplateRolloutDialogFocusesName(t *testing.T) {
	ctx := updateDetailCtx{Name: "148", TufMetadata: releaseTufMetadata(api.UpdateTufResp{})}
	html := renderUpdateHTML(t, ctx)
	start := strings.Index(html, `<dialog id="rolloutModal"`)
	if start == -1 {
		t.Fatal("rendered page does not contain the rollout dialog")
	}
	end := strings.Index(html[start:], "</dialog>")
	if end == -1 {
		t.Fatal("rendered rollout dialog is not closed")
	}
	rolloutDialog := html[start : start+end]

	assert.Contains(t, rolloutDialog, `id="rollout-name" name="name" autofocus`)
	assert.Equal(t, 1, strings.Count(rolloutDialog, "autofocus"))
}

func TestUpdateTemplateDevicesDialogUsesModalFocus(t *testing.T) {
	ctx := updateDetailCtx{
		Name:        "148",
		Summary:     api.UpdateSummary{Status: map[string]int{"in-sync": 2}},
		TufMetadata: releaseTufMetadata(api.UpdateTufResp{}),
	}

	html := renderUpdateHTML(t, ctx)
	assertDevicesDialogUsesModalFocus(t, html)

	start := strings.Index(html, `<dialog id="devicesModal"`)
	if start == -1 {
		t.Fatal("rendered page does not contain the devices dialog")
	}
	end := strings.Index(html[start:], "</dialog>")
	if end == -1 {
		t.Fatal("rendered devices dialog is not closed")
	}
	devicesDialog := html[start : start+end]

	assert.Contains(t, devicesDialog, `class="rollout-dialog"`)
	assert.Contains(t, devicesDialog, `<div class="dlg-header">`)
	assert.Contains(t, devicesDialog, `<h2 id="devices-dlg-title" tabindex="-1">`)
	assert.Contains(t, devicesDialog, `class="dlg-close"`)
	assert.Contains(t, devicesDialog, `<div class="dlg-body" id="devicesContent">`)
	assert.Contains(t, devicesDialog, `<div class="dlg-footer">`)
	assert.Contains(t, devicesDialog, `class="dlg-btn dlg-btn-cancel"`)
}

func TestUpdateRolloutTemplateRendersScanFirstLedger(t *testing.T) {
	ctx := updateRolloutDetailCtx{
		Tag:     "main",
		Name:    "148",
		Rollout: "production",
		Details: api.Rollout{
			Uuids:  []string{"device-direct"},
			Groups: []string{"factory", "beta"},
			Effect: []string{"device-direct", "device-group"},
			Commit: true,
		},
	}

	html := renderUpdateRolloutHTML(t, ctx)

	assert.Contains(t, html, `class="rollout-detail"`)
	assert.Contains(t, html, `href="/updates/148"`)
	assert.Contains(t, html, `aria-current="page">production</span>`)
	assert.Contains(t, html, `class="btn-ghost rollout-detail__progress" href="/updates/148/rollouts/production/tail"`)
	assert.NotContains(t, html, `class="btn-solid rollout-detail__progress"`)
	assert.Contains(t, html, `>2 groups<`)
	assert.Contains(t, html, `>1 direct UUID<`)
	assert.Contains(t, html, `>2 devices<`)
	assert.Contains(t, html, `>Scheduled<`)
	assert.Contains(t, html, `>main<`)
	assert.Contains(t, html, `href="/devices/device-direct"`)
	assert.Contains(t, html, `href="/devices/device-group"`)
	assert.NotContains(t, html, `id="devicesModal"`)
	assert.NotContains(t, html, `showDevicesModal`)
}

func TestUpdateRolloutTemplateRendersSummaryCountsAsText(t *testing.T) {
	ctx := updateRolloutDetailCtx{
		Name:    "148",
		Rollout: "production",
		Details: api.Rollout{Commit: true},
		Summary: api.UpdateSummary{Status: map[string]int{
			"in-sync": 1,
			"pending": 2,
		}},
	}

	html := renderUpdateRolloutHTML(t, ctx)

	assert.Contains(t, html, `>Rollout summary<`)
	assert.Contains(t, html, `>in-sync<`)
	assert.Contains(t, html, `>1 device</span>`)
	assert.Contains(t, html, `>pending<`)
	assert.Contains(t, html, `>2 devices</span>`)
	assert.NotContains(t, html, `href="#scheduled-devices"`)
	assert.Less(t, strings.Index(html, ">Rollout summary<"), strings.Index(html, ">Request and resolution<"))
}

func TestUpdateRolloutTemplatePreparingState(t *testing.T) {
	ctx := updateRolloutDetailCtx{
		Tag:     "main",
		Name:    "148",
		Rollout: "production",
		Details: api.Rollout{
			Groups: []string{"factory"},
			Effect: []string{"device-pending"},
		},
	}

	html := renderUpdateRolloutHTML(t, ctx)

	assert.Contains(t, html, `>Preparing<`)
	assert.Contains(t, html, "Recipients are still being resolved.")
	assert.NotContains(t, html, `class="recipient-list"`)
	assert.NotContains(t, html, `href="/devices/device-pending"`)
}

func TestUpdateRolloutTemplateCommittedWithoutRecipients(t *testing.T) {
	ctx := updateRolloutDetailCtx{
		Tag:     "main",
		Name:    "148",
		Rollout: "production",
		Details: api.Rollout{Commit: true},
	}

	html := renderUpdateRolloutHTML(t, ctx)

	assert.Contains(t, html, `>Scheduled<`)
	assert.Contains(t, html, "None provided")
	assert.Contains(t, html, "No devices qualified for this rollout.")
}

func TestUpdateRolloutTemplatePreservesLongValues(t *testing.T) {
	longTag := strings.Repeat("tag-", 40)
	longName := strings.Repeat("update-", 30)
	longRollout := strings.Repeat("rollout-", 30)
	longUuid := strings.Repeat("uuid-", 40)
	longGroup := strings.Repeat("group-", 35)
	longEffect := strings.Repeat("device-", 30)
	ctx := updateRolloutDetailCtx{
		Tag:     longTag,
		Name:    longName,
		Rollout: longRollout,
		Details: api.Rollout{
			Uuids:  []string{longUuid},
			Groups: []string{longGroup},
			Effect: []string{longEffect},
			Commit: true,
		},
	}

	html := renderUpdateRolloutHTML(t, ctx)

	for _, value := range []string{longTag, longName, longRollout, longUuid, longGroup, longEffect} {
		assert.Contains(t, html, value)
	}
}

func TestUpdateTemplateNoApplications(t *testing.T) {
	ctx := updateDetailCtx{
		Name:         "148",
		LatestTarget: &latestTarget{Name: "main/148", Version: "148"},
		TufMetadata:  releaseTufMetadata(api.UpdateTufResp{}),
	}
	html := renderUpdateHTML(t, ctx)
	assert.Contains(t, html, "No applications included in this release")
}

func TestUpdateTemplateLongValuesPreserved(t *testing.T) {
	hash := strings.Repeat("f", 64)
	uri := "registry.example.com/" + strings.Repeat("x", 120) + "@sha256:" + strings.Repeat("a", 64)
	ctx := updateDetailCtx{
		Name: "148",
		LatestTarget: &latestTarget{
			Name:   "main/148",
			Sha256: hash,
			Apps:   []releaseApplication{{Name: "nginx", URI: uri}},
		},
		TufMetadata: releaseTufMetadata(api.UpdateTufResp{}),
	}
	html := renderUpdateHTML(t, ctx)
	assert.Contains(t, html, hash)
	assert.Contains(t, html, `data-hash="`+uri+`"`)
}

func targetsTuf(targets map[string]any) api.UpdateTufResp {
	return api.UpdateTufResp{
		"targets.json": api.UpdateTufItemResp{
			"signed": map[string]any{
				"targets": targets,
			},
		},
	}
}

func TestFindLatestTargetComplete(t *testing.T) {
	tuf := targetsTuf(map[string]any{
		"main/148": map[string]any{
			"hashes": map[string]any{"sha256": "aaa"},
			"custom": map[string]any{
				"version":     "148",
				"arch":        "amd64",
				"hardwareIds": []any{"intel-corei7-64"},
				"tags":        []any{"main"},
				"docker_compose_apps": map[string]any{
					"shellhttpd": map[string]any{"uri": "registry/shellhttpd@sha256:aaa"},
					"nginx":      map[string]any{"uri": "registry/nginx@sha256:bbb"},
				},
			},
		},
	})

	got := findLatestTarget(tuf)
	if got == nil {
		t.Fatal("findLatestTarget() = nil, want a target")
	}
	if got.Architecture != "amd64" {
		t.Errorf("Architecture = %q, want amd64", got.Architecture)
	}
	if !reflect.DeepEqual(got.HardwareIDs, []string{"intel-corei7-64"}) {
		t.Errorf("HardwareIDs = %#v, want [intel-corei7-64]", got.HardwareIDs)
	}
	if !reflect.DeepEqual(got.Tags, []string{"main"}) {
		t.Errorf("Tags = %#v, want [main]", got.Tags)
	}
	want := []releaseApplication{
		{Name: "nginx", URI: "registry/nginx@sha256:bbb"},
		{Name: "shellhttpd", URI: "registry/shellhttpd@sha256:aaa"},
	}
	if !reflect.DeepEqual(got.Apps, want) {
		t.Errorf("Apps = %#v, want %#v", got.Apps, want)
	}
}

func TestFindLatestTargetPicksHighestVersion(t *testing.T) {
	tuf := targetsTuf(map[string]any{
		"main/1":  map[string]any{"custom": map[string]any{"version": "1"}},
		"main/10": map[string]any{"custom": map[string]any{"version": "10"}},
		"main/2":  map[string]any{"custom": map[string]any{"version": "2"}},
	})

	got := findLatestTarget(tuf)
	if got == nil || got.Name != "main/10" {
		t.Fatalf("findLatestTarget() = %#v, want name main/10", got)
	}
}

func TestFindLatestTargetSkipsMalformedVersion(t *testing.T) {
	tuf := targetsTuf(map[string]any{
		"main/bad": map[string]any{"custom": map[string]any{"version": "not-a-number"}},
		"main/5":   map[string]any{"custom": map[string]any{"version": "5"}},
	})

	got := findLatestTarget(tuf)
	if got == nil || got.Name != "main/5" {
		t.Fatalf("findLatestTarget() = %#v, want name main/5", got)
	}
}

func TestFindLatestTargetMissingTargetsJson(t *testing.T) {
	if got := findLatestTarget(api.UpdateTufResp{}); got != nil {
		t.Errorf("findLatestTarget() = %#v, want nil", got)
	}
}

func TestFindLatestTargetEmptyApps(t *testing.T) {
	tuf := targetsTuf(map[string]any{
		"main/1": map[string]any{"custom": map[string]any{"version": "1"}},
	})

	got := findLatestTarget(tuf)
	if got == nil {
		t.Fatal("findLatestTarget() = nil, want a target")
	}
	if len(got.Apps) != 0 {
		t.Errorf("Apps = %#v, want empty", got.Apps)
	}
}

func TestReleaseTufMetadataOrderAndMissingRoles(t *testing.T) {
	tuf := api.UpdateTufResp{
		"targets.json": api.UpdateTufItemResp{
			"signed": map[string]any{"expires": "2030-01-01T00:00:00Z"},
		},
		"root.json": api.UpdateTufItemResp{
			"signed": map[string]any{"expires": "2031-01-01T00:00:00Z"},
		},
	}

	got := releaseTufMetadata(tuf)
	want := []tufMetadataItem{
		{Name: "Root", Expires: "2031-01-01T00:00:00Z"},
		{Name: "Timestamp", Expires: ""},
		{Name: "Snapshot", Expires: ""},
		{Name: "Targets", Expires: "2030-01-01T00:00:00Z"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("releaseTufMetadata() = %#v, want %#v", got, want)
	}
}

func TestRolloutSummaryFromPreparing(t *testing.T) {
	got := rolloutSummaryFrom("r1", api.Rollout{Commit: false, Effect: []string{"a", "b"}})
	want := rolloutSummary{Name: "r1", State: "Preparing", DeviceCount: 2}
	if got != want {
		t.Errorf("rolloutSummaryFrom() = %#v, want %#v", got, want)
	}
}

func TestRolloutSummaryFromScheduled(t *testing.T) {
	got := rolloutSummaryFrom("r1", api.Rollout{Commit: true, Uuids: []string{"x", "y", "z"}, Effect: []string{"a"}})
	want := rolloutSummary{Name: "r1", State: "Scheduled", DeviceCount: 1}
	if got != want {
		t.Errorf("rolloutSummaryFrom() = %#v, want %#v", got, want)
	}
}
