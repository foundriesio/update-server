// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/secure-systems-lab/go-securesystemslib/cjson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/foundriesio/update-server/clock"
	appctx "github.com/foundriesio/update-server/context"
	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/tuf"
)

// newTufStorage returns a Storage with an initialized and loaded TUF setup,
// ready for AddTarget operations.
func newTufStorage(t *testing.T) *Storage {
	t.Helper()
	tmpdir := t.TempDir()
	dbFile := filepath.Join(tmpdir, "sql.db")
	db, err := storage.NewDb(dbFile)
	require.NoError(t, err)
	fs, err := storage.NewFs(tmpdir)
	require.NoError(t, err)

	require.NoError(t, fs.Auth.InitHmacSecret())
	require.NoError(t, fs.Tuf.InitTuf())
	require.NoError(t, fs.Tuf.LoadTuf())

	s, err := NewStorage(db, fs)
	require.NoError(t, err)
	return s
}

// readTufMeta reads and unmarshals a TUF metadata file written by
// GenerateTufMeta into tufDir.
func readTufMeta(t *testing.T, tufDir, name string, v any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(tufDir, name))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, v))
}

// verifyTufSig asserts that sig is a valid signature over signed, produced by a
// key listed in the latest root metadata.
func verifyTufSig(t *testing.T, s *Storage, sig tuf.Signature, signed any) {
	t.Helper()
	require.Equal(t, tuf.SigEd25519, sig.Method)

	roots, err := s.fs.Tuf.GetRoots()
	require.NoError(t, err)
	require.NotEmpty(t, roots)
	root := roots[len(roots)-1]

	key, ok := root.Signed.Keys[sig.KeyID]
	require.True(t, ok, "signature key %s not present in root metadata", sig.KeyID)
	pub, err := hex.DecodeString(key.KeyValue.Public)
	require.NoError(t, err)

	msg, err := cjson.EncodeCanonical(signed)
	require.NoError(t, err)
	require.True(t, ed25519.Verify(ed25519.PublicKey(pub), msg, sig.Signature),
		"signature must verify against canonical signed payload")
}

func TestAddTarget(t *testing.T) {
	fixedNow := time.Date(2026, time.June, 25, 12, 0, 0, 0, time.UTC)
	clock.Now = func() time.Time { return fixedNow }
	defer func() { clock.Now = time.Now }()

	s := newTufStorage(t)

	const tag = "main"
	opts := TargetOptions{
		AppVersion: 3,
		HardwareId: "raspberrypi4-64",
		Name:       "raspberrypi4-64-lmp",
		OstreeHash: "deadbeef",
		Tag:        tag,
		Apps: map[string]string{
			"shellhttpd": "sha256hashvalue",
		},
		BaseUrl: "https://example.com",
	}
	tufDir := filepath.Join(t.TempDir(), "tuf")
	require.NoError(t, s.GenerateTufMeta(tufDir, opts))

	// Targets metadata is written, with the expected version and one target.
	var targets tuf.AtsTufTargets
	readTufMeta(t, tufDir, storage.TufTargetsFile, &targets)
	assert.Equal(t, "Targets", targets.Signed.Type)
	assert.Equal(t, 1, targets.Signed.Version)
	assert.Equal(t, fixedNow.Add(s.fs.Tuf.TargetsExpiration).Truncate(time.Second), targets.Signed.Expires)
	require.Len(t, targets.Signed.Targets, 1)

	targetName := "raspberrypi4-64-lmp-3"
	target, ok := targets.Signed.Targets[targetName]
	require.True(t, ok, "expected target %q", targetName)
	assert.Equal(t, int64(0), target.Length)
	assert.Equal(t, "deadbeef", hex.EncodeToString(target.Hashes["sha256"]))

	// The custom block carries the generated target metadata.
	var custom generatedTargetCustom
	require.NoError(t, json.Unmarshal(target.Custom, &custom))
	assert.Equal(t, "raspberrypi4-64-lmp", custom.Name)
	assert.Equal(t, "3", custom.Version)
	assert.Equal(t, []string{"raspberrypi4-64"}, custom.HardwareIds)
	assert.Equal(t, "OSTREE", custom.TargetFormat)
	assert.Equal(t, fixedNow.Format(time.RFC3339), custom.CreatedAt)
	require.Len(t, custom.DockerComposeApps, 1)
	assert.Equal(t, "https://example.com/composeapphack/shellhttpd@sha256:sha256hashvalue", custom.DockerComposeApps["shellhttpd"].URI)
	// Targets metadata is signed by the targets role.
	require.Len(t, targets.Signatures, 1)
	verifyTufSig(t, s, targets.Signatures[0], targets.Signed)

	// Snapshot metadata references the targets version and is signed.
	var snapshot tuf.AtsTufSnapshot
	readTufMeta(t, tufDir, storage.TufSnapshotFile, &snapshot)
	assert.Equal(t, "Snapshot", snapshot.Signed.Type)
	assert.Equal(t, targets.Signed.Version, snapshot.Signed.Version)
	assert.Equal(t, targets.Signed.Expires, snapshot.Signed.Expires)
	assert.Equal(t, targets.Signed.Version, snapshot.Signed.Meta[storage.TufTargetsFile].Version)
	require.Len(t, snapshot.Signatures, 1)
	verifyTufSig(t, s, snapshot.Signatures[0], snapshot.Signed)

	// Timestamp metadata references the snapshot version and uses the reserved
	// (version * 1000) numbering scheme.
	var timestamp tuf.AtsTufTimestamp
	readTufMeta(t, tufDir, storage.TufTimestampFile, &timestamp)
	assert.Equal(t, "Timestamp", timestamp.Signed.Type)
	assert.Equal(t, targets.Signed.Version*1000, timestamp.Signed.Version)
	assert.Equal(t, fixedNow.Add(s.fs.Tuf.TimestampExpiration).Truncate(time.Second), timestamp.Signed.Expires)
	assert.Equal(t, snapshot.Signed.Version, timestamp.Signed.Meta[storage.TufSnapshotFile].Version)
	require.Len(t, timestamp.Signatures, 1)
	verifyTufSig(t, s, timestamp.Signatures[0], timestamp.Signed)
}

func TestAddTargetWithoutApps(t *testing.T) {
	s := newTufStorage(t)

	const tag = "main"
	tufDir := filepath.Join(t.TempDir(), "tuf")
	require.NoError(t, s.GenerateTufMeta(tufDir, TargetOptions{
		AppVersion: 5,
		HardwareId: "intel-corei7-64",
		Name:       "intel-corei7-64-lmp",
		OstreeHash: "abc123",
		Tag:        tag,
	}))

	var targets tuf.AtsTufTargets
	readTufMeta(t, tufDir, storage.TufTargetsFile, &targets)
	require.Len(t, targets.Signed.Targets, 1)

	target := targets.Signed.Targets["intel-corei7-64-lmp-5"]
	var custom generatedTargetCustom
	require.NoError(t, json.Unmarshal(target.Custom, &custom))
	assert.Equal(t, "5", custom.Version)
	assert.Nil(t, custom.DockerComposeApps, "docker_compose_apps should be omitted when there are no apps")
}

func TestAddTargetIncrementsTufVersion(t *testing.T) {
	s := newTufStorage(t)

	const tag = "main"

	// Seed an existing update whose targets metadata is at TUF version 5.
	existing := tuf.AtsTufTargets{
		Signed: tuf.TargetsMeta{
			SignedCommon: tuf.SignedCommon{
				Type:    tuf.RoleTargets.TufType(),
				Version: 5,
			},
			Targets: tuf.TargetFiles{
				"prev-target-10": tuf.TargetFileMeta{
					Custom: json.RawMessage(`{"version":10}`),
				},
			},
		},
	}
	existingJSON, err := json.Marshal(existing)
	require.NoError(t, err)
	require.NoError(t, s.InsertUpdate(tag, "update-5", "tester"))
	require.NoError(t, s.fs.Updates.Tuf.WriteFile(tag, "update-5", storage.TufTargetsFile, string(existingJSON)))

	tufDir := filepath.Join(t.TempDir(), "tuf")
	// A new target should bump the TUF version above the existing one.
	require.NoError(t, s.GenerateTufMeta(tufDir, TargetOptions{
		HardwareId: "intel-corei7-64",
		Name:       "intel-corei7-64-lmp",
		OstreeHash: "abc123",
		Tag:        tag,
	}))

	var targets tuf.AtsTufTargets
	readTufMeta(t, tufDir, storage.TufTargetsFile, &targets)
	assert.Equal(t, 6, targets.Signed.Version, "TUF version should be ten greater than the existing update")

	var timestamp tuf.AtsTufTimestamp
	readTufMeta(t, tufDir, storage.TufTimestampFile, &timestamp)
	assert.Equal(t, 6000, timestamp.Signed.Version)
}

// writeUpdateTimestamp registers an update in the database and writes a
// timestamp.json with the given version and expiry into its TUF directory.
func writeUpdateTimestamp(t *testing.T, s *Storage, tag, update string, version int, expires time.Time) {
	t.Helper()
	ts := tuf.AtsTufTimestamp{
		Signed: tuf.TimestampMeta{
			SignedCommon: tuf.SignedCommon{
				Type:    tuf.RoleTimestamp.TufType(),
				Version: version,
				Expires: expires,
			},
			Meta: map[string]tuf.MetaItem{
				storage.TufSnapshotFile: {Version: 1},
			},
		},
	}
	tsJSON, err := json.Marshal(ts)
	require.NoError(t, err)
	require.NoError(t, s.InsertUpdate(tag, update, "tester"))
	require.NoError(t, s.fs.Tuf.WriteTimestamp(tag, update, tsJSON))
}

func TestRefreshTufTimestamps(t *testing.T) {
	fixedNow := time.Date(2026, time.June, 26, 12, 0, 0, 0, time.UTC)
	clock.Now = func() time.Time { return fixedNow }
	defer func() { clock.Now = time.Now }()

	s := newTufStorage(t)

	const tag = "main"
	// One timestamp expires within the 1-day cutoff and should be refreshed.
	soonExpiry := fixedNow.Add(12 * time.Hour)
	writeUpdateTimestamp(t, s, tag, "update-soon", 1000, soonExpiry)
	// One timestamp is well in the future and should be left untouched.
	laterExpiry := fixedNow.Add(30 * 24 * time.Hour).Truncate(time.Second)
	writeUpdateTimestamp(t, s, tag, "update-later", 2000, laterExpiry)

	ctx := appctx.CtxWithLog(appctx.Background(), slog.Default())
	require.NoError(t, s.RefreshTufTimestamps(ctx))

	// The soon-to-expire timestamp was re-signed with a fresh expiry.
	var refreshed tuf.AtsTufTimestamp
	require.NoError(t, s.fs.Tuf.ReadTufMeta(tag, "update-soon", storage.TufTimestampFile, &refreshed))
	expectedExpiry := fixedNow.Add(s.fs.Tuf.TimestampExpiration).Truncate(time.Second)
	assert.Equal(t, expectedExpiry, refreshed.Signed.Expires)
	assert.Equal(t, 1000, refreshed.Signed.Version, "refresh should not change the timestamp version")
	require.Len(t, refreshed.Signatures, 1)
	verifyTufSig(t, s, refreshed.Signatures[0], refreshed.Signed)

	// The future timestamp was left unchanged.
	var untouched tuf.AtsTufTimestamp
	require.NoError(t, s.fs.Tuf.ReadTufMeta(tag, "update-later", storage.TufTimestampFile, &untouched))
	assert.Equal(t, laterExpiry, untouched.Signed.Expires)
	assert.Empty(t, untouched.Signatures, "future timestamp should not be re-signed")
}
