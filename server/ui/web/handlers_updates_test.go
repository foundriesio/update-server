// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/foundriesio/update-server/server/ui/api"
	"github.com/foundriesio/update-server/server/ui/web/templates"
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

func assertDevicesDialogUsesModalFocus(t *testing.T, html string) {
	t.Helper()
	assert.Contains(t, html, `<dialog id="devicesModal"`)
	assert.Contains(t, html, `aria-labelledby="devices-dlg-title"`)
	assert.Contains(t, html, `id="devices-dlg-title" tabindex="-1"`)
	assert.Contains(t, html, `showDevicesModal(`)
	assert.Contains(t, html, `, this); return false;`)
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

func TestUpdateRolloutTemplateDevicesDialogUsesModalFocus(t *testing.T) {
	ctx := updateRolloutDetailCtx{
		Name:    "148",
		Rollout: "production",
		Summary: api.UpdateSummary{Status: map[string]int{"in-sync": 2}},
	}

	assertDevicesDialogUsesModalFocus(t, renderUpdateRolloutHTML(t, ctx))
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
