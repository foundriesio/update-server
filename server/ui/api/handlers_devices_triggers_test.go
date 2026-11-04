// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/users"
	"github.com/stretchr/testify/require"
)

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
	require.NoError(t, json.Unmarshal(tc.POST("/devices/foo/remote-actions", 200, tc.marshalBody(RemoteActionTrigger{Action: "diag"})), &result))
	require.True(t, storage.TestIdRegex.MatchString(result.TestID))
	require.Contains(t, result.TestID, "diag_")

	tc.POST("/devices/foo/remote-actions", 400, tc.marshalBody(RemoteActionTrigger{Action: ""}))
	tc.POST("/devices/foo/remote-actions", 400, tc.marshalBody(RemoteActionTrigger{Action: "../diag"}))
	tc.POST("/devices/foo/remote-actions", 400, tc.marshalBody(RemoteActionTrigger{Action: "invalid.action"}))
	tc.POST("/devices/foo/remote-actions", 400, strings.NewReader(`{"action":"diag"}{"action":"reboot"}`))
	var saved storage.ConfigFileSet
	require.NoError(t, json.Unmarshal(tc.GET("/configs/device/foo", 200), &saved))
	require.Equal(t, "preserve me", saved.Files["existing"].Value)
	require.Equal(t, "COMMAND_ID="+result.TestID+"\nCOMMAND_CAPTURE=1\n", saved.Files["fioconfig-oneshot-diag"].Value)
	require.Equal(t, []string{"/usr/share/fioconfig/handlers/fioconfig-oneshot", "diag"}, saved.Files["fioconfig-oneshot-diag"].OnChanged)
}
