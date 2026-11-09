// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/foundriesio/update-server/auth"
	"github.com/foundriesio/update-server/clock"
	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/users"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestRemoteActionStatesSameSecondUpdates(t *testing.T) {
	fixedNow := time.Date(2026, time.June, 25, 12, 0, 0, 0, time.UTC)
	clock.Now = func() time.Time { return fixedNow }
	defer func() { clock.Now = time.Now }()

	tc := NewTestClient(t)
	device, err := tc.gw.DeviceCreate("foo", "cert1")
	require.NoError(t, err)
	tc.u.AllowedScopes = users.ScopeDevicesRU | users.ScopeUpdatesRU
	tc.PUT("/configs/device/foo", 204, storage.ConfigFileSet{Files: map[string]storage.ConfigFile{
		"fio-remote-actions": {Value: "diag,reboot"},
	}})
	trigger := func(action string) string {
		t.Helper()
		var result RemoteActionTriggerResult
		body := tc.POST("/devices/foo/remote-actions", 200, tc.marshalBody(RemoteActionTrigger{Action: action}), echo.HeaderContentType, echo.MIMEApplicationJSON)
		require.NoError(t, json.Unmarshal(body, &result))
		return result.TestID
	}
	read := func() RemoteActions {
		t.Helper()
		var result RemoteActions
		require.NoError(t, json.Unmarshal(tc.GET("/devices/foo/remote-actions", 200), &result))
		return result
	}
	diag := trigger("diag")
	reboot := trigger("reboot")
	tc.u.AllowedScopes = users.ScopeDevicesR
	require.Equal(t, []RemoteActionState{{Action: "diag", TestID: diag, State: "pending"}, {Action: "reboot", TestID: reboot, State: "pending"}}, read().Outstanding)
	var saved storage.ConfigFileSet
	require.NoError(t, json.Unmarshal(tc.GET("/configs/device/foo", 200), &saved))
	applied := storage.AppliedConfigs{Files: saved.Files, AppliedAt: saved.CreatedAt}
	require.NoError(t, device.SaveAppliedConfigs(applied))
	require.Equal(t, []RemoteActionState{{Action: "diag", TestID: diag, State: "queued"}, {Action: "reboot", TestID: reboot, State: "queued"}}, read().Outstanding)
	require.NoError(t, device.TestCreate("target", "diag", diag))
	require.Equal(t, []RemoteActionState{{Action: "reboot", TestID: reboot, State: "queued"}}, read().Outstanding)
	require.NoError(t, device.TestComplete(diag, "PASSED", "", nil))
	tc.u.AllowedScopes = users.ScopeDevicesRU | users.ScopeUpdatesRU
	newDiag := trigger("diag")
	require.NotEqual(t, diag, newDiag)
	history, err := tc.api.ReadDeviceConfigHistory("foo", 10, true)
	require.NoError(t, err)
	require.Len(t, history, 4)
	for _, item := range history {
		require.Equal(t, fixedNow.Unix(), item.CreatedAt)
	}
	require.Equal(t, applied.AppliedAt, history[0].CreatedAt)
	require.Contains(t, history[0].RawFiles, newDiag)
	require.Contains(t, history[1].RawFiles, diag)
	require.Equal(t, []RemoteActionState{{Action: "diag", TestID: newDiag, State: "pending"}, {Action: "reboot", TestID: reboot, State: "queued"}}, read().Outstanding)
	// Retained commands remain visible even after the action menu is removed.
	require.NoError(t, json.Unmarshal(tc.GET("/configs/device/foo", 200), &saved))
	delete(saved.Files, "fio-remote-actions")
	tc.PUT("/configs/device/foo", 204, saved)
	tc.u.AllowedScopes = users.ScopeDevicesR
	require.Empty(t, read().Actions)
	require.Len(t, read().Outstanding, 2)
	require.NoError(t, device.TestCreate("target", "diag", newDiag))
	require.NoError(t, device.TestCreate("target", "reboot", reboot))
	require.Empty(t, read().Outstanding)
}

func TestRemoteActionReadErrors(t *testing.T) {
	for _, corrupt := range []string{"history", "command", "delivery", "tests"} {
		t.Run(corrupt, func(t *testing.T) {
			tc := NewTestClient(t)
			_, err := tc.gw.DeviceCreate("foo", "cert1")
			require.NoError(t, err)
			tc.u.AllowedScopes = users.ScopeDevicesR
			files := map[string]storage.ConfigFile{"fioconfig-oneshot-diag": {
				Value: "COMMAND_ID=diag_123456789abcdef\nCOMMAND_CAPTURE=1\n", Unencrypted: true,
				OnChanged: []string{"/usr/share/fioconfig/handlers/fioconfig-oneshot", "diag"},
			}}
			if corrupt == "command" {
				files["fioconfig-oneshot-diag"] = storage.ConfigFile{Value: "invalid"}
			}
			content, err := json.Marshal(files)
			require.NoError(t, err)
			if corrupt == "history" {
				content = []byte("invalid")
			}
			require.NoError(t, tc.api.SaveDeviceConfig("foo", string(content), "test", "test"))
			if corrupt == "delivery" {
				require.NoError(t, tc.fs.Devices.WriteFile("foo", storage.ConfigAppliedFile, "invalid"))
			}
			if corrupt == "tests" {
				require.NoError(t, tc.fs.Devices.WriteFile("foo", storage.TestsPrefix+"diag_123456789abcdef", "invalid"))
			}
			tc.GET("/devices/foo/remote-actions", 500)
		})
	}
}

func TestRemoteActionIOErrors(t *testing.T) {
	for _, source := range []string{"history", "delivery", "tests"} {
		t.Run(source, func(t *testing.T) {
			tc := NewTestClient(t)
			_, err := tc.gw.DeviceCreate("foo", "cert1")
			require.NoError(t, err)
			tc.u.AllowedScopes = users.ScopeDevicesR
			files := remoteActionTestConfig("diag_123456789abcdef")
			content, err := json.Marshal(files)
			require.NoError(t, err)
			require.NoError(t, tc.api.SaveDeviceConfig("foo", string(content), "test", "test"))
			require.NoError(t, tc.fs.Devices.WriteFile("foo", "existing", "preserve me"))
			var path, diagnostic string
			switch source {
			case "history":
				path = filepath.Join(tc.fs.Config.ConfigsDir(), storage.ConfigsDeviceDir, "foo", storage.ConfigsJournalFile)
				require.NoError(t, os.Remove(path))
				diagnostic = "Failed to read device config"
			case "delivery":
				path = filepath.Join(tc.fs.Config.DevicesDir(), "foo", storage.ConfigAppliedFile)
				diagnostic = "Failed to read delivered remote actions"
			case "tests":
				path = filepath.Join(tc.fs.Config.DevicesDir(), "foo", storage.TestsPrefix+"diag_123456789abcdef")
				diagnostic = "Failed to read remote action tests"
			}
			// A directory at a file path fails reads even when tests run as root.
			require.NoError(t, os.Mkdir(path, 0o750))
			body := tc.GET("/devices/foo/remote-actions", 500)
			require.Contains(t, string(body), diagnostic)
			require.NotContains(t, string(body), `"outstanding"`)
		})
	}
}

func remoteActionTestConfig(id string) map[string]storage.ConfigFile {
	return map[string]storage.ConfigFile{"fioconfig-oneshot-diag": {
		Value: "COMMAND_ID=" + id + "\nCOMMAND_CAPTURE=1\n", Unencrypted: true,
		OnChanged: []string{"/usr/share/fioconfig/handlers/fioconfig-oneshot", "diag"},
	}}
}

func readRemoteActionsDuringDelivery(t *testing.T, tc *testClient, applied storage.AppliedConfigs, between func()) RemoteActions {
	t.Helper()
	content, err := json.Marshal(applied)
	require.NoError(t, err)
	path := filepath.Join(tc.fs.Config.DevicesDir(), "foo", storage.ConfigAppliedFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	// The FIFO handshake pauses the handler after history, before delivery/tests.
	require.NoError(t, syscall.Mkfifo(path, 0o600))
	type openedFile struct {
		file *os.File
		err  error
	}
	opened := make(chan openedFile, 1)
	go func() {
		file, err := os.OpenFile(path, os.O_WRONLY, 0)
		opened <- openedFile{file, err}
	}()
	done := make(chan bool)
	rec := tc.DoAsync(httptest.NewRequest(http.MethodGet, "/v1/devices/foo/remote-actions", nil), done)
	select {
	case result := <-opened:
		require.NoError(t, result.err)
		defer result.file.Close() //nolint:errcheck
		between()
		_, err := result.file.Write(content)
		require.NoError(t, err)
		require.NoError(t, result.file.Close())
	case <-time.After(5 * time.Second):
		// Release the writer if a regression prevents the handler reaching delivery.
		file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		require.NoError(t, err)
		result := <-opened
		if result.file != nil {
			_ = result.file.Close()
		}
		_ = file.Close()
		t.Fatal("handler did not reach the delivered config read")
	}
	tc.waitDone(done)
	require.Equal(t, http.StatusOK, rec.Code())
	var result RemoteActions
	require.NoError(t, json.Unmarshal([]byte(rec.BodyString()), &result))
	return result
}

func TestRemoteActionReadInterleavings(t *testing.T) {
	for _, report := range []bool{false, true} {
		name := "history replaced before delivery"
		if report {
			name = "test reported before delivery completes"
		}
		t.Run(name, func(t *testing.T) {
			tc := NewTestClient(t)
			device, err := tc.gw.DeviceCreate("foo", "cert1")
			require.NoError(t, err)
			tc.u.AllowedScopes = users.ScopeDevicesR
			oldID, newID := "diag_123456789abcdef", "diag_abcdef123456789"
			files := remoteActionTestConfig(oldID)
			content, err := json.Marshal(files)
			require.NoError(t, err)
			require.NoError(t, tc.api.SaveDeviceConfig("foo", string(content), "test", "test"))
			applied := storage.AppliedConfigs{Files: remoteActionTestConfig(newID)}
			result := readRemoteActionsDuringDelivery(t, tc, applied, func() {
				content, err := json.Marshal(applied.Files)
				require.NoError(t, err)
				require.NoError(t, tc.api.SaveDeviceConfig("foo", string(content), "test", "test"))
				if report {
					require.NoError(t, device.TestCreate("target", "diag", oldID))
				} else {
					require.NoError(t, device.TestCreate("target", "diag", newID))
				}
			})
			if report {
				require.Empty(t, result.Outstanding)
			} else {
				require.Equal(t, []RemoteActionState{{Action: "diag", TestID: oldID, State: "pending"}}, result.Outstanding)
			}
			require.NoError(t, os.Remove(filepath.Join(tc.fs.Config.DevicesDir(), "foo", storage.ConfigAppliedFile)))
			require.NoError(t, device.SaveAppliedConfigs(applied))
			result = RemoteActions{}
			require.NoError(t, json.Unmarshal(tc.GET("/devices/foo/remote-actions", 200), &result))
			if report {
				require.Equal(t, []RemoteActionState{{Action: "diag", TestID: newID, State: "queued"}}, result.Outstanding)
			} else {
				require.Empty(t, result.Outstanding)
			}
		})
	}
}

func TestRemoteActionCommands(t *testing.T) {
	id := "diag_123456789abcdef"
	for _, test := range []struct {
		name, value string
		valid       bool
	}{
		{"id only", "COMMAND_ID=" + id, true},
		{"capture disabled", "COMMAND_ID=" + id + "\nCOMMAND_CAPTURE=0\n", true},
		{"capture enabled", "COMMAND_ID=" + id + "\nCOMMAND_CAPTURE=1\n", true},
		{"reordered fields", "COMMAND_CAPTURE=1\nCOMMAND_ID=" + id + "\nEXTRA=1\n", true},
		{"missing assignment", id, false},
		{"missing id", "COMMAND_CAPTURE=1\n", false},
		{"empty id", "COMMAND_ID=\n", false},
		{"invalid id", "COMMAND_ID=../diag", false},
		{"different action", "COMMAND_ID=reboot_123456789abcdef", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			configs := map[string]storage.ConfigFile{"fioconfig-oneshot-diag": {Value: test.value}}
			commands, err := remoteActionCommands(configs)
			if !test.valid {
				require.ErrorContains(t, err, "invalid remote action command in fioconfig-oneshot-diag")
				return
			}
			require.NoError(t, err)
			require.Equal(t, []RemoteActionState{{Action: "diag", TestID: id}}, commands)
			require.Equal(t, []RemoteActionState{{Action: "diag", TestID: id, State: "pending"}}, outstandingRemoteActions(commands, configs, nil, nil))
			applied := &storage.AppliedConfigs{Files: configs}
			require.Equal(t, []RemoteActionState{{Action: "diag", TestID: id, State: "queued"}}, outstandingRemoteActions(commands, configs, applied, nil))
			require.Empty(t, outstandingRemoteActions(commands, configs, applied, []storage.TargetTest{{Uuid: id}}))
		})
	}
}

func TestRemoteActionDeliveredContent(t *testing.T) {
	file := storage.ConfigFile{
		Value: "COMMAND_ID=diag_123456789abcdef\nCOMMAND_CAPTURE=1\n", Unencrypted: true,
		OnChanged: []string{"/usr/share/fioconfig/handlers/fioconfig-oneshot", "diag"},
	}
	configs := map[string]storage.ConfigFile{"fioconfig-oneshot-diag": file}
	commands, err := remoteActionCommands(configs)
	require.NoError(t, err)
	for _, field := range []string{"id", "value", "encryption", "handler", "absent"} {
		t.Run(field, func(t *testing.T) {
			delivered := file
			switch field {
			case "id":
				delivered.Value = "COMMAND_ID=diag_abcdef123456789\nCOMMAND_CAPTURE=1\n"
			case "value":
				delivered.Value += "EXTRA=1\n"
			case "encryption":
				delivered.Unencrypted = false
			case "handler":
				delivered.OnChanged = []string{"other", "diag"}
			case "absent":
				delivered = storage.ConfigFile{}
			}
			applied := &storage.AppliedConfigs{Files: map[string]storage.ConfigFile{"fioconfig-oneshot-diag": delivered}, AppliedAt: 9999999999}
			states := outstandingRemoteActions(commands, configs, applied, nil)
			require.Equal(t, "pending", states[0].State)
		})
	}
}

func TestRemoteActionCsrf(t *testing.T) {
	tc := NewTestClient(t)
	// Production installs CSRF middleware outside the API handler registration.
	tc.e.Use(auth.CsrfCheck)
	_, err := tc.gw.DeviceCreate("foo", "cert1")
	require.NoError(t, err)
	tc.u.AllowedScopes = users.ScopeDevicesRU | users.ScopeUpdatesRU
	require.NoError(t, tc.api.SaveDeviceConfig("foo", `{"fio-remote-actions":{"Value":"diag"}}`, "test", "test"))
	for _, test := range []struct {
		name, cookie, token string
		status              int
	}{
		{"missing cookie", "", "token", 403},
		{"missing token", "token", "", 403},
		{"mismatch", "token", "other", 403},
		{"valid", "token", "token", 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/devices/foo/remote-actions", strings.NewReader(`{"action":"diag"}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			if test.cookie != "" {
				req.AddCookie(&http.Cookie{Name: auth.CsrfCookieName, Value: test.cookie})
			}
			req.Header.Set(auth.CsrfHeaderName, test.token)
			require.Equal(t, test.status, tc.Do(req).Code)
			history, err := tc.api.ReadDeviceConfigHistory("foo", 10, true)
			require.NoError(t, err)
			if test.status == 403 {
				require.Len(t, history, 1)
			} else {
				require.Len(t, history, 2)
			}
		})
	}
}

func TestRemoteActionValidationBoundary(t *testing.T) {
	tc := NewTestClient(t)
	_, err := tc.gw.DeviceCreate("foo", "cert1")
	require.NoError(t, err)
	tc.u.AllowedScopes = users.ScopeDevicesRU | users.ScopeUpdatesRU
	valid, invalid := strings.Repeat("a", 32), strings.Repeat("b", 33)
	tc.PUT("/configs/device/foo", 204, storage.ConfigFileSet{Files: map[string]storage.ConfigFile{
		"fio-remote-actions": {Value: " , " + valid + ", " + invalid},
	}})
	var actions RemoteActions
	require.NoError(t, json.Unmarshal(tc.GET("/devices/foo/remote-actions", 200), &actions))
	require.Equal(t, []string{valid}, actions.Actions)
	body := tc.POST("/devices/foo/remote-actions", 200, tc.marshalBody(RemoteActionTrigger{Action: valid}), echo.HeaderContentType, echo.MIMEApplicationJSON)
	var result RemoteActionTriggerResult
	require.NoError(t, json.Unmarshal(body, &result))
	require.Len(t, result.TestID, 48)
	require.True(t, storage.TestIdRegex.MatchString(result.TestID))
	tc.POST("/devices/foo/remote-actions", 400, tc.marshalBody(RemoteActionTrigger{Action: invalid}), echo.HeaderContentType, echo.MIMEApplicationJSON)
	tc.POST("/devices/foo/remote-actions", 400, strings.NewReader(`{}`), echo.HeaderContentType, echo.MIMEApplicationJSON)
}

func TestValidRemoteAction(t *testing.T) {
	require.True(t, validRemoteAction("diag"))
	require.True(t, validRemoteAction(strings.Repeat("a", 32)))
	require.False(t, validRemoteAction(strings.Repeat("a", 33)))
	require.False(t, validRemoteAction("invalid.action"))
	require.False(t, validRemoteAction("../diag"))
}

func TestDeviceRemoteActions(t *testing.T) {
	tc := NewTestClient(t)
	_, err := tc.gw.DeviceCreate("foo", "cert1")
	require.NoError(t, err)

	tc.GET("/devices/foo/remote-actions", 403)
	tc.u.AllowedScopes = users.ScopeDevicesR
	require.JSONEq(t, `{"actions":[]}`, string(tc.GET("/devices/foo/remote-actions", 200)))

	tc.u.AllowedScopes = users.ScopeDevicesRU | users.ScopeUpdatesRU
	configs := storage.ConfigFileSet{
		Reason: "configure remote actions",
		Files: map[string]storage.ConfigFile{
			"fio-remote-actions": {Value: " reboot, diag, reboot, invalid.action "},
			"existing":           {Value: "preserve me"},
		},
	}
	tc.PUT("/configs/device/foo", 204, configs)

	tc.u.AllowedScopes = users.ScopeDevicesR
	require.JSONEq(t, `{"actions":["diag","reboot"]}`, string(tc.GET("/devices/foo/remote-actions", 200)))
	tc.POST("/devices/foo/remote-actions", 403, tc.marshalBody(RemoteActionTrigger{Action: "diag"}))

	tc.u.AllowedScopes = users.ScopeDevicesRU | users.ScopeUpdatesRU
	var result RemoteActionTriggerResult
	require.NoError(t, json.Unmarshal(tc.POST("/devices/foo/remote-actions", 200, tc.marshalBody(RemoteActionTrigger{Action: "diag"}), echo.HeaderContentType, echo.MIMEApplicationJSON), &result))
	require.True(t, storage.TestIdRegex.MatchString(result.TestID))
	require.Contains(t, result.TestID, "diag_")

	for _, action := range []string{"", "../diag", "invalid.action", "unconfigured"} {
		body := tc.POST("/devices/foo/remote-actions", 400, tc.marshalBody(RemoteActionTrigger{Action: action}), echo.HeaderContentType, echo.MIMEApplicationJSON)
		require.Contains(t, string(body), "is not configured for this device")
		var diagnostic map[string]string
		require.NoError(t, json.Unmarshal(body, &diagnostic))
		require.Contains(t, diagnostic["message"], action)
	}
	tc.POST("/devices/foo/remote-actions", 400, strings.NewReader(`{"action":`), echo.HeaderContentType, echo.MIMEApplicationJSON)
	var saved storage.ConfigFileSet
	require.NoError(t, json.Unmarshal(tc.GET("/configs/device/foo", 200), &saved))
	require.Equal(t, "preserve me", saved.Files["existing"].Value)
	require.Equal(t, "COMMAND_ID="+result.TestID+"\nCOMMAND_CAPTURE=1\n", saved.Files["fioconfig-oneshot-diag"].Value)
	require.Equal(t, []string{"/usr/share/fioconfig/handlers/fioconfig-oneshot", "diag"}, saved.Files["fioconfig-oneshot-diag"].OnChanged)
	require.True(t, saved.Files["fioconfig-oneshot-diag"].Unencrypted)
	// Bind follows adjacent mutations: unknown fields and trailing JSON are accepted.
	tc.POST("/devices/foo/remote-actions", 200, strings.NewReader(`{"action":"diag","extra":true}{"action":"reboot"}`), echo.HeaderContentType, echo.MIMEApplicationJSON)
}
