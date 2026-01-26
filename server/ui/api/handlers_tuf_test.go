// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/foundriesio/update-server/storage/tuf"
	"github.com/foundriesio/update-server/storage/users"
)

func TestApiTufRoot(t *testing.T) {
	tc := NewTestClient(t)

	// Requires the updates:read scope.
	tc.GET("/tuf/root.json", 403)
	tc.GET("/tuf/1.root.json", 403)
	tc.u.AllowedScopes = users.ScopeUpdatesR

	// Before TUF is initialized there is no root metadata.
	tc.GET("/tuf/root.json", 404)

	require.Nil(t, tc.fs.Tuf.InitTuf())

	// The latest root.json is returned and is valid v1 root metadata.
	var root tuf.AtsTufRoot
	require.Nil(t, json.Unmarshal(tc.GET("/tuf/root.json", 200), &root))
	require.Equal(t, "Root", root.Signed.Type)
	require.Equal(t, 1, root.Signed.Version)
	require.Len(t, root.Signatures, 1)

	// The explicit version returns the same document.
	var byVersion tuf.AtsTufRoot
	require.Nil(t, json.Unmarshal(tc.GET("/tuf/1.root.json", 200), &byVersion))
	require.Equal(t, root.Signed.Version, byVersion.Signed.Version)
	require.Equal(t, root.Signatures, byVersion.Signatures)

	// Unknown versions and malformed names are 404.
	tc.GET("/tuf/2.root.json", 404)
	tc.GET("/tuf/not-a-version", 404)
	tc.GET("/tuf/0.root.json", 404)
}
