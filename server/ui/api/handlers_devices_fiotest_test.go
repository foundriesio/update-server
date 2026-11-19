// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/foundriesio/update-server/storage/users"
	"github.com/stretchr/testify/require"
)

func TestDeviceTestArtifactTraversal(t *testing.T) {
	tc := NewTestClient(t)
	device, err := tc.gw.DeviceCreate("foo", "cert1")
	require.NoError(t, err)
	tc.u.AllowedScopes = users.ScopeDevicesR
	const testID = "diag_123456789abcdef"
	require.NoError(t, device.TestCreate("target", "diag", testID))
	require.NoError(t, device.TestStoreArtifact(testID, "output.txt", strings.NewReader("test output")))
	require.NoError(t, tc.fs.Devices.WriteFile("foo", "secret.txt", "private device data"))
	require.NoError(t, tc.fs.Devices.WriteFile("other", "secret.txt", "private other device data"))

	base := "/v1/devices/foo/tests/" + testID + "/"
	rec := tc.Do(httptest.NewRequest(http.MethodGet, base+"output.txt", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "test output", rec.Body.String())

	for _, artifact := range []string{
		"../secret.txt",
		"/../secret.txt",
		"/../../other/secret.txt",
		"%2f..%2fsecret.txt",
		"%2F..%2F..%2Fother%2Fsecret.txt",
		"%2f%2e%2e%2fsecret.txt",
		"%252f%252e%252e%252fsecret.txt",
		`..\secret.txt`,
		"%5c..%5csecret.txt",
	} {
		t.Run(artifact, func(t *testing.T) {
			rec := tc.Do(httptest.NewRequest(http.MethodGet, base+artifact, nil))
			require.Equal(t, http.StatusNotFound, rec.Code)
			require.NotContains(t, rec.Body.String(), "private device data")
			require.NotContains(t, rec.Body.String(), "private other device data")
		})
	}
}
