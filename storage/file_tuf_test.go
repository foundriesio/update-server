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

func TestRefreshUpdateTuf_noRefreshNeeded(t *testing.T) {
	fs := newTufFs(t)
	h, err := storage.InitTuf(fs)
	require.NoError(t, err)

	updateDir := t.TempDir()
	tufDir := filepath.Join(updateDir, "tuf")
	require.NoError(t, os.MkdirAll(tufDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(tufDir, "targets.json"), minimalTargetsJSON(t), 0o640))
	require.NoError(t, h.ProcessUpdateTuf(tufDir, updateDir, nil))

	// Read back snapshot/timestamp versions before refresh attempt.
	snapBefore := readSnapshot(t, tufDir)
	tsBefore := readTimestamp(t, tufDir)

	// Threshold of 1 nanosecond — files expire in ~1 year so no refresh should happen.
	refreshed, err := h.RefreshUpdateTuf(tufDir, time.Nanosecond)
	require.NoError(t, err)
	require.False(t, refreshed)

	// Versions should be unchanged.
	require.Equal(t, snapBefore.Signed.Version, readSnapshot(t, tufDir).Signed.Version)
	require.Equal(t, tsBefore.Signed.Version, readTimestamp(t, tufDir).Signed.Version)
}

func TestRefreshUpdateTuf_refreshNeeded(t *testing.T) {
	fs := newTufFs(t)
	h, err := storage.InitTuf(fs)
	require.NoError(t, err)

	updateDir := t.TempDir()
	tufDir := filepath.Join(updateDir, "tuf")
	require.NoError(t, os.MkdirAll(tufDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(tufDir, "targets.json"), minimalTargetsJSON(t), 0o640))
	require.NoError(t, h.ProcessUpdateTuf(tufDir, updateDir, nil))

	snapBefore := readSnapshot(t, tufDir)
	tsBefore := readTimestamp(t, tufDir)

	// Threshold of 2 years — files expire in ~1 year, so refresh should trigger.
	refreshed, err := h.RefreshUpdateTuf(tufDir, 2*365*24*time.Hour)
	require.NoError(t, err)
	require.True(t, refreshed)

	snapAfter := readSnapshot(t, tufDir)
	tsAfter := readTimestamp(t, tufDir)

	require.Equal(t, snapBefore.Signed.Version+1, snapAfter.Signed.Version)
	require.Equal(t, tsBefore.Signed.Version+1, tsAfter.Signed.Version)
	require.True(t, snapAfter.Signed.Expires.After(snapBefore.Signed.Expires))
	require.True(t, tsAfter.Signed.Expires.After(tsBefore.Signed.Expires))
	// Timestamp must reference the new snapshot version.
	require.Equal(t, snapAfter.Signed.Version, tsAfter.Signed.Meta["snapshot.json"].Version)
	// Signatures must be updated.
	require.NotEqual(t, snapBefore.Signatures[0].Sig, snapAfter.Signatures[0].Sig)
	require.NotEqual(t, tsBefore.Signatures[0].Sig, tsAfter.Signatures[0].Sig)
}

func minimalTargetsJSON(t *testing.T) []byte {
	t.Helper()
	return []byte(`{"signatures":[],"signed":{"_type":"Targets","expires":"2020-01-01T00:00:00Z","version":1,"targets":{"x-1":{"length":1,"hashes":{"sha256":"abc"},"custom":{"tags":["main"]}}}}}`)
}

func readSnapshot(t *testing.T, tufDir string) storage.TufSnapshot {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(tufDir, "snapshot.json"))
	require.NoError(t, err)
	var s storage.TufSnapshot
	require.NoError(t, json.Unmarshal(data, &s))
	return s
}

func readTimestamp(t *testing.T, tufDir string) storage.TufTimestamp {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(tufDir, "timestamp.json"))
	require.NoError(t, err)
	var ts storage.TufTimestamp
	require.NoError(t, json.Unmarshal(data, &ts))
	return ts
}

func TestProcessUpdateTuf(t *testing.T) {
	fs := newTufFs(t)
	h, err := storage.InitTuf(fs)
	require.NoError(t, err)

	// Create a minimal update directory simulating an uploaded update.
	updateDir := t.TempDir()
	tufDir := filepath.Join(updateDir, "tuf")
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

	require.NoError(t, h.ProcessUpdateTuf(tufDir, updateDir, nil))

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

func TestProcessUpdateTuf_fromParams(t *testing.T) {
	fs := newTufFs(t)
	h, err := storage.InitTuf(fs)
	require.NoError(t, err)

	updateDir := t.TempDir()
	tufDir := filepath.Join(updateDir, "tuf")

	// Set up an ostree ref and two compose apps.
	ostreeRefsDir := filepath.Join(updateDir, "ostree_repo", "refs", "heads")
	require.NoError(t, os.MkdirAll(ostreeRefsDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(ostreeRefsDir, "intel-corei7-64-lmp"), []byte("deadbeef1234"), 0o640))

	appDir := filepath.Join(updateDir, "apps", "apps", "myapp", "abc123sha")
	require.NoError(t, os.MkdirAll(appDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "uri"), []byte(""), 0o640))

	params := &storage.TufTargetParams{
		HardwareID: "intel-corei7-64",
		Version:    "42",
		Tag:        "main",
		BaseURI:    "https://example.com",
	}
	require.NoError(t, h.ProcessUpdateTuf(tufDir, updateDir, params))

	// targets.json should be generated server-side.
	targetsData, err := os.ReadFile(filepath.Join(tufDir, "targets.json"))
	require.NoError(t, err)
	var targets storage.TufTargets
	require.NoError(t, json.Unmarshal(targetsData, &targets))

	// Name defaults to ostree branch name.
	require.Contains(t, targets.Signed.Targets, "intel-corei7-64-lmp-42")
	target := targets.Signed.Targets["intel-corei7-64-lmp-42"]
	require.NotNil(t, target.Custom)
	require.Equal(t, []string{"intel-corei7-64"}, target.Custom.HardwareIds)
	require.Equal(t, "42", target.Custom.Version)
	require.Equal(t, []string{"main"}, target.Custom.Tags)
	require.Equal(t, "deadbeef1234", target.Hashes.Sha256)
	require.Contains(t, target.Custom.ComposeApps, "myapp")
	require.Equal(t, "https://example.com/apps/myapp/abc123sha", target.Custom.ComposeApps["myapp"].Uri)

	// snapshot.json and timestamp.json should be generated.
	_, err = os.ReadFile(filepath.Join(tufDir, "snapshot.json"))
	require.NoError(t, err)
	_, err = os.ReadFile(filepath.Join(tufDir, "timestamp.json"))
	require.NoError(t, err)
}
