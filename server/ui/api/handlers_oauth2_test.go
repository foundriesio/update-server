// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/foundriesio/update-server/storage/users"
)

func TestApiOAuth2DeviceApprove(t *testing.T) {
	tc := NewTestClient(t)

	// The approver must be a stored user so the approval can be recorded.
	approver := users.User{Username: "approver", AllowedScopes: users.ScopeDevicesC | users.ScopeDevicesR}
	require.NoError(t, tc.users.Create(&approver))
	stored, err := tc.users.Get("approver")
	require.NoError(t, err)
	*tc.u = *stored
	post := func(resource string, status int, body any) {
		tc.POST(resource, status, tc.marshalBody(body), "Content-Type", "application/json")
	}

	now := time.Now().Unix()
	deviceCode, userCode, err := tc.users.CreateDeviceAuth(now+600, now+3600, "devices:create,devices:read,updates:read")
	require.NoError(t, err)

	// Lookup shows what approving would grant: the requested scopes the caller holds.
	var info DeviceAuthInfo
	require.NoError(t, json.Unmarshal(tc.GET("/oauth2/device/"+userCode, 200), &info))
	require.Equal(t, userCode, info.UserCode)
	require.Equal(t, (users.ScopeDevicesC | users.ScopeDevicesR).String(), info.Scopes)
	tc.GET("/oauth2/device/ZZZZ-ZZZZ", 404)

	post("/oauth2/device/approve", 400, DeviceApproveRequest{})
	post("/oauth2/device/approve", 404, DeviceApproveRequest{UserCode: "ZZZZ-ZZZZ"})
	post("/oauth2/device/approve", 400, DeviceApproveRequest{UserCode: userCode, Scopes: "bogus"})
	post("/oauth2/device/approve", 200, DeviceApproveRequest{UserCode: userCode, Description: "unoq-b", Scopes: "devices:create"})
	post("/oauth2/device/approve", 400, DeviceApproveRequest{UserCode: userCode})
	tc.GET("/oauth2/device/"+userCode, 400)

	auth, err := tc.users.GetDeviceAuthByDeviceCode(deviceCode)
	require.NoError(t, err)
	require.True(t, auth.Authorized)
	require.Equal(t, "unoq-b", auth.TokenDescription)
	// Narrowed by the request's scopes.
	require.Equal(t, users.ScopeDevicesC.String(), auth.Scopes)

	// Without a scope limit, the grant is the requested scopes the caller holds.
	fullCode, fullUserCode, err := tc.users.CreateDeviceAuth(now+600, now+3600, "devices:create,devices:read,updates:read")
	require.NoError(t, err)
	post("/oauth2/device/approve", 200, DeviceApproveRequest{UserCode: fullUserCode})
	auth, err = tc.users.GetDeviceAuthByDeviceCode(fullCode)
	require.NoError(t, err)
	require.Equal(t, (users.ScopeDevicesC | users.ScopeDevicesR).String(), auth.Scopes)

	_, expiredCode, err := tc.users.CreateDeviceAuth(now-1, now+3600, "devices:create")
	require.NoError(t, err)
	post("/oauth2/device/approve", 400, DeviceApproveRequest{UserCode: expiredCode})

	// Nothing the caller holds was requested.
	_, otherCode, err := tc.users.CreateDeviceAuth(now+600, now+3600, "users:read")
	require.NoError(t, err)
	tc.GET("/oauth2/device/"+otherCode, 403)
	post("/oauth2/device/approve", 403, DeviceApproveRequest{UserCode: otherCode})

	// fiocli devices approve limits the grant to devices:create.
	_, loginCode, err := tc.users.CreateDeviceAuth(now+600, now+3600, "devices:read")
	require.NoError(t, err)
	post("/oauth2/device/approve", 400, DeviceApproveRequest{UserCode: loginCode, Scopes: "devices:create"})
}

func TestApiOAuth2DeviceDeny(t *testing.T) {
	tc := NewTestClient(t)

	denier := users.User{Username: "denier", AllowedScopes: users.ScopeDevicesR}
	require.NoError(t, tc.users.Create(&denier))
	stored, err := tc.users.Get("denier")
	require.NoError(t, err)
	*tc.u = *stored

	now := time.Now().Unix()
	deviceCode, userCode, err := tc.users.CreateDeviceAuth(now+600, now+3600, "devices:create")
	require.NoError(t, err)

	body := func(code string) io.Reader { return tc.marshalBody(DeviceDenyRequest{UserCode: code}) }
	tc.POST("/oauth2/device/deny", 404, body("ZZZZ-ZZZZ"), "Content-Type", "application/json")
	// Denying needs none of the requested scopes.
	tc.POST("/oauth2/device/deny", 200, body(userCode), "Content-Type", "application/json")
	tc.POST("/oauth2/device/deny", 400, body(userCode), "Content-Type", "application/json")

	auth, err := tc.users.GetDeviceAuthByDeviceCode(deviceCode)
	require.NoError(t, err)
	require.True(t, auth.Denied)
}
