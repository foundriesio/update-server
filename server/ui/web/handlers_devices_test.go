// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/foundriesio/update-server/auth"
	"github.com/foundriesio/update-server/context"
	"github.com/foundriesio/update-server/server/ui/api"
	"github.com/foundriesio/update-server/server/ui/web/templates"
	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/users"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type remoteActionRenderCase struct {
	name, state, testStatus string
	canTrigger, removed     bool
	unavailable, multiple   bool
	want                    string
}

func newRemoteActionBackend(t *testing.T, test remoteActionRenderCase) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		var result any
		switch r.URL.Path {
		case "/v1/devices/device":
			result = api.Device{Uuid: "device"}
		case "/v1/devices/device/tests":
			result = []storage.TargetTest{}
			if test.testStatus != "" {
				result = []storage.TargetTest{{Uuid: "diag_123456789abcdef", Status: test.testStatus}}
			}
		case "/v1/devices/device/remote-actions":
			if test.unavailable {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			actions := api.RemoteActions{Actions: []string{"diag"}}
			if test.removed {
				actions.Actions = nil
			}
			if test.state != "" {
				actions.Outstanding = []api.RemoteActionState{{Action: "diag", TestID: "diag_123456789abcdef", State: test.state}}
			}
			if test.multiple {
				actions.Outstanding = append(actions.Outstanding, api.RemoteActionState{Action: "reboot", TestID: "reboot_123456789abcdef", State: "queued"})
			}
			result = actions
		default:
			t.Errorf("unexpected API request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		assert.NoError(t, json.NewEncoder(w).Encode(result))
	}))
}

func serveDeviceRequest(e *echo.Echo, session *auth.Session, req *http.Request) *httptest.ResponseRecorder {
	ctx := context.CtxWithLog(req.Context(), slog.New(slog.DiscardHandler))
	req = req.WithContext(CtxWithSession(ctx, session))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestDevicesTestsRemoteActionStates(t *testing.T) {
	for _, test := range []remoteActionRenderCase{
		{name: "pending", state: "pending", canTrigger: true, want: "Pending delivery to the device."},
		{name: "read only pending", state: "pending", want: "Pending delivery to the device."},
		{name: "queued", state: "queued", canTrigger: true, want: "Results have not been reported."},
		{name: "mixed commands", state: "pending", multiple: true, want: "Pending delivery to the device."},
		{name: "removed menu", state: "queued", removed: true, want: "Results have not been reported."},
		{name: "running", state: "queued", testStatus: "RUNNING", canTrigger: true, want: "RUNNING"},
		{name: "passed", state: "pending", testStatus: "PASSED", want: "PASSED"},
		{name: "no commands", canTrigger: true},
		{name: "unavailable", unavailable: true, want: "Remote actions are temporarily unavailable."},
	} {
		t.Run(test.name, func(t *testing.T) {
			id := "diag_123456789abcdef"
			backend := newRemoteActionBackend(t, test)
			defer backend.Close()
			scopes := users.ScopeDevicesR
			if test.canTrigger {
				scopes = users.ScopeDevicesRU | users.ScopeUpdatesRU
			}
			session := &auth.Session{BaseUrl: backend.URL, Client: backend.Client(), User: &users.User{AllowedScopes: scopes}}
			e := echo.New()
			h := handlers{templates: templates.Templates}
			e.GET("/devices/:uuid/tests", h.devicesTests)
			var plain string
			for _, query := range []string{"", "?remote-action-submitted=1"} {
				req := httptest.NewRequest(http.MethodGet, "/devices/device/tests"+query, nil)
				req.AddCookie(&http.Cookie{Name: auth.CsrfCookieName, Value: "csrf-token"})
				rec := serveDeviceRequest(e, session, req)
				require.Equal(t, http.StatusOK, rec.Code)
				html := rec.Body.String()
				if query == "" {
					plain = html
				} else {
					assert.Equal(t, plain, html)
				}
				if test.want != "" {
					assert.Contains(t, html, test.want)
				}
				if test.multiple {
					assert.Contains(t, html, "Results have not been reported.")
					assert.Greater(t, strings.Index(html, "reboot_123456789abcdef"), strings.Index(html, id))
				}
				if test.testStatus != "" || test.state == "" {
					assert.NotContains(t, html, `class="remote-action-progress"`)
				} else {
					assert.Contains(t, html, id)
					assert.Contains(t, html, `<span>Refresh</span>`)
					assert.Contains(t, html, `href="/devices/device/tests"`)
					assert.Contains(t, html, `title="Refresh test results" aria-label="Refresh test results"`)
				}
				if test.canTrigger && !test.unavailable {
					assert.Contains(t, html, `name="_csrf" value="csrf-token"`)
					assert.Contains(t, html, `data-dialog-open="remote-action-dialog"`)
				} else {
					assert.NotContains(t, html, `data-dialog-open="remote-action-dialog"`)
				}
			}
		})
	}
}

func TestRemoteActionFormCsrfAndRedirect(t *testing.T) {
	calls := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, "/v1/devices/device/remote-actions", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "token", r.Header.Get(auth.CsrfHeaderName))
		cookie, err := r.Cookie(auth.CsrfCookieName)
		assert.NoError(t, err)
		if cookie != nil {
			assert.Equal(t, "token", cookie.Value)
		}
		var trigger api.RemoteActionTrigger
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&trigger))
		assert.Equal(t, "diag", trigger.Action)
		assert.NoError(t, json.NewEncoder(w).Encode(api.RemoteActionTriggerResult{TestID: "diag_123456789abcdef"}))
	}))
	defer backend.Close()
	session := &auth.Session{BaseUrl: backend.URL, Client: backend.Client(), User: &users.User{AllowedScopes: users.ScopeDevicesRU | users.ScopeUpdatesRU}}
	e := echo.New()
	h := handlers{}
	e.POST("/devices/:uuid/remote-actions", h.devicesRemoteActionTrigger, h.requireScope(users.ScopeDevicesRU|users.ScopeUpdatesRU), auth.CsrfCheck)
	for _, test := range []struct {
		name, cookie, token string
		status              int
	}{
		{"missing cookie", "", "token", 403},
		{"missing token", "token", "", 403},
		{"mismatch", "token", "other", 403},
		{"valid", "token", "token", 303},
	} {
		t.Run(test.name, func(t *testing.T) {
			form := url.Values{"action": {"diag"}, "_csrf": {test.token}}
			req := httptest.NewRequest(http.MethodPost, "/devices/device/remote-actions", strings.NewReader(form.Encode()))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
			if test.cookie != "" {
				req.AddCookie(&http.Cookie{Name: auth.CsrfCookieName, Value: test.cookie})
			}
			rec := serveDeviceRequest(e, session, req)
			require.Equal(t, test.status, rec.Code)
			if test.status == 303 {
				assert.Equal(t, "/devices/device/tests", rec.Header().Get(echo.HeaderLocation))
				assert.Equal(t, 1, calls)
			} else {
				assert.Zero(t, calls)
			}
		})
	}
}

func boolPtr(b bool) *bool { return &b }

func TestRemoteActionDialogSafeguards(t *testing.T) {
	asset, err := templates.Assets.ReadFile("device_tests.html")
	require.NoError(t, err)
	html := string(asset)
	for _, expected := range []string{
		`<script type="module">`,
		`let submitInFlight = false;`,
		`if (submitInFlight) {`,
		`event.preventDefault();`,
		`submitButton.disabled = true;`,
		`submitButton.setAttribute('aria-busy', 'true');`,
		`form.setAttribute('aria-busy', 'true');`,
		`button.disabled = true;`,
		`document.getElementById('remote-action-title').focus();`,
		`activeTrigger.focus();`,
		`window.addEventListener('pageshow', (event) => {`,
		`submitButton.disabled = false;`,
		`button.disabled = false;`,
		`for="remote-action-value"`,
		`<select class="field-input remote-action-select"`,
		`name="_csrf" value="{{.CsrfToken}}"`,
		`Actions run on the device and report results in the Tests view.`,
		`<span>Refresh</span>`,
	} {
		assert.Contains(t, html, expected)
	}
	for _, forbidden := range []string{"setInterval", "setTimeout", "remote-action-submitted", "checks in", "check-in", "Action submitted.", "Refresh the page to see the results."} {
		assert.NotContains(t, html, forbidden)
	}
}

func TestUnreportedRemoteActions(t *testing.T) {
	commands := []api.RemoteActionState{
		{Action: "diag", TestID: "diag_123456789abcdef", State: "pending"},
		{Action: "reboot", TestID: "reboot_123456789abcdef", State: "queued"},
	}
	// An older invocation must not clear a newer command during overlapping reads.
	tests := []storage.TargetTest{{Uuid: "diag_abcdef123456789", Status: "PASSED"}}
	assert.Equal(t, commands, unreportedRemoteActions(commands, tests))
	tests = append(tests, storage.TargetTest{Uuid: commands[0].TestID, Status: "RUNNING"})
	assert.Equal(t, commands[1:], unreportedRemoteActions(commands, tests))
}

func TestRemoteActionFormBinding(t *testing.T) {
	form := url.Values{"action": {"diag"}, "_csrf": {"token"}}
	req := httptest.NewRequest(http.MethodPost, "/devices/device/remote-actions", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	ctx := echo.New().NewContext(req, httptest.NewRecorder())

	var trigger api.RemoteActionTrigger
	if err := ctx.Bind(&trigger); err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	if trigger.Action != "diag" {
		t.Errorf("bound action = %q, want diag", trigger.Action)
	}
}

func TestBuildUpdateStepsHappyPath(t *testing.T) {
	events := []storage.DeviceUpdateEvent{
		{DeviceTime: "2026-08-30T09:00:27Z", Event: storage.DeviceEvent{Success: boolPtr(true)}},
		{DeviceTime: "2026-08-30T09:00:28Z", Event: storage.DeviceEvent{Success: boolPtr(true)}},
		{DeviceTime: "2026-08-30T09:00:31Z", Event: storage.DeviceEvent{Success: boolPtr(true)}},
	}

	steps, duration, failed := buildUpdateSteps(events)

	if failed {
		t.Errorf("failed = true, want false")
	}
	if duration != "4s" {
		t.Errorf("duration = %q, want 4s", duration)
	}
	want := []string{"+0s", "+1s", "+3s"}
	for i, w := range want {
		if steps[i].Elapsed != w {
			t.Errorf("steps[%d].Elapsed = %q, want %q", i, steps[i].Elapsed, w)
		}
	}
}

func TestBuildUpdateStepsMarksFailure(t *testing.T) {
	events := []storage.DeviceUpdateEvent{
		{DeviceTime: "2026-08-30T09:00:27Z", Event: storage.DeviceEvent{Success: boolPtr(true)}},
		{DeviceTime: "2026-08-30T09:00:28Z", Event: storage.DeviceEvent{Success: boolPtr(false)}},
	}

	steps, _, failed := buildUpdateSteps(events)

	if !failed {
		t.Errorf("failed = false, want true")
	}
	if steps[0].Failed {
		t.Errorf("steps[0].Failed = true, want false")
	}
	if !steps[1].Failed {
		t.Errorf("steps[1].Failed = false, want true")
	}
}

func TestBuildUpdateStepsClampsNegativeElapsed(t *testing.T) {
	events := []storage.DeviceUpdateEvent{
		{DeviceTime: "2026-08-30T09:00:27Z", Event: storage.DeviceEvent{Success: boolPtr(true)}},
		{DeviceTime: "2026-08-30T09:00:22Z", Event: storage.DeviceEvent{Success: boolPtr(true)}},
	}

	steps, duration, _ := buildUpdateSteps(events)

	if steps[1].Elapsed != "+0s" {
		t.Errorf("steps[1].Elapsed = %q, want +0s", steps[1].Elapsed)
	}
	if duration != "0s" {
		t.Errorf("duration = %q, want 0s", duration)
	}
}

func TestBuildTestStatsCountsPassedAndFailed(t *testing.T) {
	tests := []storage.TargetTest{
		{Status: "PASSED"},
		{Status: "FAILED"},
		{Status: "RUNNING"},
	}

	total, passed, failed := buildTestStats(tests)

	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if passed != 1 {
		t.Errorf("passed = %d, want 1", passed)
	}
	if failed != 1 {
		t.Errorf("failed = %d, want 1", failed)
	}
}

func TestBuildTestRowsComputesDurationOnlyWhenCompleted(t *testing.T) {
	completed := int64(1700000045)
	tests := []storage.TargetTest{
		{CreatedOn: 1700000000, CompletedOn: &completed, Status: "PASSED"},
		{CreatedOn: 1700000000, Status: "RUNNING"},
	}

	rows := buildTestRows(tests)

	if rows[0].Duration != "00:45" {
		t.Errorf("rows[0].Duration = %q, want 00:45", rows[0].Duration)
	}
	if rows[1].Duration != "" {
		t.Errorf("rows[1].Duration = %q, want empty for a running test", rows[1].Duration)
	}
	if rows[1].CompletedShort != "" {
		t.Errorf("rows[1].CompletedShort = %q, want empty for a running test", rows[1].CompletedShort)
	}
}

func TestBuildUpdateStepsNilSuccessIsNotFailure(t *testing.T) {
	events := []storage.DeviceUpdateEvent{
		{DeviceTime: "2026-08-30T09:00:27Z", Event: storage.DeviceEvent{}},
	}

	steps, duration, failed := buildUpdateSteps(events)

	if failed {
		t.Errorf("failed = true, want false")
	}
	if steps[0].Failed {
		t.Errorf("steps[0].Failed = true, want false")
	}
	if duration != "0s" {
		t.Errorf("duration = %q, want 0s", duration)
	}
}
