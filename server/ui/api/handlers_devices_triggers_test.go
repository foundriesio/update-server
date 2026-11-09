// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/foundriesio/update-server/auth"
	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/users"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

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
