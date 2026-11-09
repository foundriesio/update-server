// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
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

	now := time.Now().Unix()
	deviceCode, userCode, err := tc.users.CreateDeviceAuth(now+600, now+3600, "devices:create,updates:read")
	require.NoError(t, err)

	tc.POST("/oauth2/device/approve", 400, tc.marshalBody(DeviceApproveRequest{}), "Content-Type", "application/json")
	tc.POST("/oauth2/device/approve", 404, tc.marshalBody(DeviceApproveRequest{UserCode: "ZZZZ-ZZZZ"}), "Content-Type", "application/json")
	tc.POST("/oauth2/device/approve", 200, tc.marshalBody(DeviceApproveRequest{UserCode: userCode, Description: "unoq-b"}), "Content-Type", "application/json")
	tc.POST("/oauth2/device/approve", 400, tc.marshalBody(DeviceApproveRequest{UserCode: userCode}), "Content-Type", "application/json")

	auth, err := tc.users.GetDeviceAuthByDeviceCode(deviceCode)
	require.NoError(t, err)
	require.True(t, auth.Authorized)
	require.Equal(t, "unoq-b", auth.TokenDescription)
	// Only devices:create is granted, whatever else the code requested.
	require.Equal(t, users.ScopeDevicesC.String(), auth.Scopes)

	_, expiredCode, err := tc.users.CreateDeviceAuth(now-1, now+3600, "devices:create")
	require.NoError(t, err)
	tc.POST("/oauth2/device/approve", 400, tc.marshalBody(DeviceApproveRequest{UserCode: expiredCode}), "Content-Type", "application/json")

	_, otherCode, err := tc.users.CreateDeviceAuth(now+600, now+3600, "users:read")
	require.NoError(t, err)
	tc.POST("/oauth2/device/approve", 400, tc.marshalBody(DeviceApproveRequest{UserCode: otherCode}), "Content-Type", "application/json")

	// The route requires the devices:create scope.
	_, regCode, err := tc.users.CreateDeviceAuth(now+600, now+3600, "devices:create")
	require.NoError(t, err)
	tc.u.AllowedScopes = users.ScopeDevicesR
	tc.POST("/oauth2/device/approve", 403, tc.marshalBody(DeviceApproveRequest{UserCode: regCode}), "Content-Type", "application/json")
}
