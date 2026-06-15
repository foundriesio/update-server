// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/foundriesio/update-server/storage"
)

func newTufFs(t *testing.T) *storage.FsHandle {
	t.Helper()
	fs, err := storage.NewFs(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, fs.Auth.InitHmacSecret())
	return fs
}

func TestInitTuf_success(t *testing.T) {
	fs := newTufFs(t)
	h, err := storage.InitTuf(fs)
	require.NoError(t, err)
	require.NotNil(t, h)
	require.NotNil(t, h.RootKey)
	require.NotNil(t, h.TimestampKey)
	require.NotNil(t, h.SnapshotKey)
	require.NotNil(t, h.TargetsKey)
}

func TestInitTuf_alreadyInitialized(t *testing.T) {
	fs := newTufFs(t)
	_, err := storage.InitTuf(fs)
	require.NoError(t, err)
	_, err = storage.InitTuf(fs)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already initialized")
}

func TestLoadTuf_notInitialized(t *testing.T) {
	fs := newTufFs(t)
	_, err := storage.LoadTuf(fs)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not initialized")
}

func TestLoadTuf_afterInit(t *testing.T) {
	fs := newTufFs(t)
	init, err := storage.InitTuf(fs)
	require.NoError(t, err)

	loaded, err := storage.LoadTuf(fs)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	// Keys loaded from disk must match what was generated.
	require.Equal(t, []byte(init.RootKey), []byte(loaded.RootKey))
	require.Equal(t, []byte(init.TimestampKey), []byte(loaded.TimestampKey))
	require.Equal(t, []byte(init.SnapshotKey), []byte(loaded.SnapshotKey))
	require.Equal(t, []byte(init.TargetsKey), []byte(loaded.TargetsKey))
}

func TestGetRoots_single(t *testing.T) {
	fs := newTufFs(t)
	h, err := storage.InitTuf(fs)
	require.NoError(t, err)

	roots, err := h.GetRoots()
	require.NoError(t, err)
	require.Len(t, roots, 1)

	root := roots[0]
	require.Equal(t, "Root", root.Signed.Type)
	require.Equal(t, 1, root.Signed.Version)

	// Expiry should be ~10 years from now (within 24h tolerance).
	expectedExpiry := time.Now().Add(10 * 365 * 24 * time.Hour)
	require.WithinDuration(t, expectedExpiry, root.Signed.Expires, 24*time.Hour)

	require.Len(t, root.Signed.Keys, 4)
	require.Len(t, root.Signed.Roles, 4)
	require.Contains(t, root.Signed.Roles, "root")
	require.Contains(t, root.Signed.Roles, "timestamp")
	require.Contains(t, root.Signed.Roles, "snapshot")
	require.Contains(t, root.Signed.Roles, "targets")

	require.Len(t, root.Signatures, 1)
	require.Equal(t, "ed25519", root.Signatures[0].Method)
	require.NotEmpty(t, root.Signatures[0].Sig)
	require.NotEmpty(t, root.Signatures[0].KeyID)
}

func TestGetRoots_keyIdsConsistent(t *testing.T) {
	fs := newTufFs(t)
	h, err := storage.InitTuf(fs)
	require.NoError(t, err)

	roots, err := h.GetRoots()
	require.NoError(t, err)
	require.Len(t, roots, 1)

	root := roots[0]
	// Every key ID referenced in every role must exist in the keys map.
	for roleName, role := range root.Signed.Roles {
		require.Len(t, role.KeyIDs, 1, "role %s should have exactly 1 key", roleName)
		keyID := role.KeyIDs[0]
		_, ok := root.Signed.Keys[keyID]
		require.True(t, ok, "key ID %s for role %s not found in keys map", keyID, roleName)
	}

	// The signature key ID must also be in the root role.
	sigKeyID := root.Signatures[0].KeyID
	rootRoleKeyID := root.Signed.Roles["root"].KeyIDs[0]
	require.Equal(t, rootRoleKeyID, sigKeyID, "signature key ID must match root role key ID")
}

func TestProcessUpdateTuf(t *testing.T) {
	fs := newTufFs(t)
	h, err := storage.InitTuf(fs)
	require.NoError(t, err)

	// Create a minimal tuf directory simulating an uploaded update.
	tufDir := filepath.Join(t.TempDir(), "tuf")
	require.NoError(t, os.MkdirAll(tufDir, 0o750))

	targetsJSON := `{
		"signatures": [{"keyid":"old","method":"ed25519","sig":"deadbeef"}],
		"signed": {
			"_type": "Targets",
			"expires": "2020-01-01T00:00:00Z",
			"version": 99,
			"targets": {
				"mydevice-42": {
					"length": 1234,
					"hashes": {"sha256": "abc123"},
					"custom": {"tags": ["main"], "name": "mydevice", "version": "42"}
				}
			}
		}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(tufDir, "targets.json"), []byte(targetsJSON), 0o640))

	require.NoError(t, h.ProcessUpdateTuf(tufDir))

	// targets.json should be re-signed with version=1 and a fresh expiry.
	targetsData, err := os.ReadFile(filepath.Join(tufDir, "targets.json"))
	require.NoError(t, err)
	var targets storage.TufTargets
	require.NoError(t, json.Unmarshal(targetsData, &targets))
	require.Equal(t, 1, targets.Signed.Version)
	require.WithinDuration(t, time.Now().Add(365*24*time.Hour), targets.Signed.Expires, 24*time.Hour)
	require.Len(t, targets.Signatures, 1)
	require.Equal(t, "ed25519", targets.Signatures[0].Method)
	require.NotEqual(t, "deadbeef", targets.Signatures[0].Sig, "old signature should be replaced")

	// snapshot.json should reference targets and root.
	snapshotData, err := os.ReadFile(filepath.Join(tufDir, "snapshot.json"))
	require.NoError(t, err)
	var snapshot storage.TufSnapshot
	require.NoError(t, json.Unmarshal(snapshotData, &snapshot))
	require.Equal(t, "Snapshot", snapshot.Signed.Type)
	require.Equal(t, 1, snapshot.Signed.Version)
	require.Contains(t, snapshot.Signed.Meta, "targets.json")
	require.Contains(t, snapshot.Signed.Meta, "root.json")
	require.Len(t, snapshot.Signatures, 1)
	require.Equal(t, "ed25519", snapshot.Signatures[0].Method)

	// timestamp.json should reference snapshot.
	timestampData, err := os.ReadFile(filepath.Join(tufDir, "timestamp.json"))
	require.NoError(t, err)
	var timestamp storage.TufTimestamp
	require.NoError(t, json.Unmarshal(timestampData, &timestamp))
	require.Equal(t, "Timestamp", timestamp.Signed.Type)
	require.Equal(t, 1, timestamp.Signed.Version)
	require.Contains(t, timestamp.Signed.Meta, "snapshot.json")
	require.Len(t, timestamp.Signatures, 1)
	require.Equal(t, "ed25519", timestamp.Signatures[0].Method)

	// 1.root.json should be symlinked into the tuf dir.
	linkTarget, err := os.Readlink(filepath.Join(tufDir, "1.root.json"))
	require.NoError(t, err)
	require.NotEmpty(t, linkTarget)
}
