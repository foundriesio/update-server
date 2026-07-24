// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

// Fixture seeding for the perf-test: registers the device rows this run's
// devices will use, seeds a minimal unsigned TUF target + a real ostree
// object for it, and assigns it to every generated device via a rollout —
// so the Locust "check for update"/"download update" tasks have something
// real to hit. Off by default (--seed-update). Implemented directly at the
// storage layer rather than through the REST API's signed-tar-upload
// endpoint: the gateway never verifies TUF signatures on GET (that's a
// TUF-client-side concern), so unsigned fixture content is sufficient here
// and far cheaper to produce.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/api"
	"github.com/foundriesio/update-server/storage/gateway"
)

// seedUpdate registers every device in uuids with the server, assigns them
// a tag, seeds a single unsigned-but-structurally-valid TUF target plus a
// real ostree object for it, and commits a rollout so GetTufMeta/
// GetOstreeFilePath resolve immediately — no server restart or async wait
// required, since this all runs before "fioserver serve" ever starts.
func seedUpdate(datadir, tag, updateName string, uuids []string, pubkeys map[string]string) error {
	fs, err := storage.NewFs(datadir)
	if err != nil {
		return fmt.Errorf("open filesystem: %w", err)
	}
	db, err := storage.NewDb(fs.Config.DbFile())
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close() //nolint:errcheck
	gw, err := gateway.NewStorage(db, fs)
	if err != nil {
		return fmt.Errorf("open gateway storage: %w", err)
	}
	ap, err := api.NewStorage(db, fs)
	if err != nil {
		return fmt.Errorf("open api storage: %w", err)
	}

	// Real mTLS registration (authDevice's DeviceCreate) happens lazily on
	// first contact. This fixture needs the rows to exist *now* so the
	// rollout below has something to match — so it registers them the same
	// way, with each device's real extracted pubkey (not a placeholder;
	// using a fake key here would make the actual Locust mTLS handshake
	// hit "Key rotation is not supported" on first contact).
	for _, uuid := range uuids {
		device, err := gw.DeviceGet(uuid)
		if err != nil {
			return fmt.Errorf("DeviceGet(%s): %w", uuid, err)
		}
		if device == nil {
			if device, err = gw.DeviceCreate(uuid, pubkeys[uuid]); err != nil {
				return fmt.Errorf("DeviceCreate(%s): %w", uuid, err)
			}
		}
		// SetUpdateName's rollout query filters on the device's current tag
		// column; a device that has never checked in has tag="" and would
		// never match a rollout for `tag`. CheckIn sets it directly, with no
		// dependency on the gateway ever having been reached.
		if err := device.CheckIn(device.TargetName, tag, device.OstreeHash, device.Apps); err != nil {
			return fmt.Errorf("CheckIn(%s): %w", uuid, err)
		}
	}

	targetName := fmt.Sprintf("%s-1", updateName)
	ostreeHash, err := writeFixtureOstreeContent(fs, tag, updateName)
	if err != nil {
		return fmt.Errorf("write ostree fixture content: %w", err)
	}

	targets, err := targetsJSON(targetName, ostreeHash)
	if err != nil {
		return fmt.Errorf("build targets.json: %w", err)
	}
	expires := time.Now().UTC().AddDate(0, 6, 0).Format(time.RFC3339)
	if err := fs.Updates.Tuf.WriteFile(tag, updateName, "targets.json", targets); err != nil {
		return fmt.Errorf("write targets.json: %w", err)
	}
	if err := fs.Updates.Tuf.WriteFile(tag, updateName, "snapshot.json", snapshotJSON(expires)); err != nil {
		return fmt.Errorf("write snapshot.json: %w", err)
	}
	if err := fs.Updates.Tuf.WriteFile(tag, updateName, "timestamp.json", timestampJSON(expires)); err != nil {
		return fmt.Errorf("write timestamp.json: %w", err)
	}
	if err := fs.Updates.Tuf.WriteFile(tag, updateName, "1.root.json", rootJSON(expires)); err != nil {
		return fmt.Errorf("write 1.root.json: %w", err)
	}

	if err := ap.InsertUpdate(tag, updateName, "perf-test"); err != nil {
		return fmt.Errorf("InsertUpdate: %w", err)
	}

	// Assign by UUID, not group: real mTLS-registered devices always have
	// group_name="" (never set at DeviceCreate time), so a rollout keyed by
	// groups would never match them.
	rollout := api.Rollout{Uuids: uuids}
	if err := ap.CreateRollout(tag, updateName, "perf-test-rollout", rollout); err != nil {
		return fmt.Errorf("CreateRollout: %w", err)
	}
	// CommitRollout's SetUpdateName is a synchronous UPDATE ... RETURNING;
	// called directly like this (rather than through the REST endpoint's
	// fire-and-forget goroutine) it takes effect immediately, no polling.
	if err := ap.CommitRollout(tag, updateName, "perf-test-rollout", rollout); err != nil {
		return fmt.Errorf("CommitRollout: %w", err)
	}

	fmt.Printf("seeded update %s/%s (target %s) and assigned it to %d device(s)\n",
		tag, updateName, targetName, len(uuids))
	return nil
}

// writeFixtureOstreeContent writes a small, real (non-empty) ostree-shaped
// object plus a matching ref, so /ostree/* download tasks stream actual
// bytes instead of a 0-byte no-op. The gateway's ostreeFileStream handler
// only does a plain os.Open/copy — it doesn't parse ostree structure server
// side — so the content itself just needs to exist at the expected path.
func writeFixtureOstreeContent(fs *storage.FsHandle, tag, updateName string) (hexHash string, err error) {
	content := make([]byte, 256*1024) // 256KiB: big enough to register as real download throughput
	for i := range content {
		content[i] = byte(i)
	}
	sum := sha256.Sum256(content)
	hexHash = hex.EncodeToString(sum[:])

	if err := fs.Updates.Ostree.WriteFile(tag, updateName, "config", "[core]\nrepo_version=1\nmode=archive-z2\n"); err != nil {
		return "", err
	}
	// WriteFile does not create intermediate subdirectories (it only
	// mkdirs the category root, e.g. .../ostree_repo — see
	// UpdatesFsHandle.updateLocalHandle), so objects/<aa>/ and refs/heads/
	// must be created explicitly before writing into them.
	objRel := filepath.Join("objects", hexHash[:2], hexHash[2:]+".filez")
	if err := os.MkdirAll(filepath.Dir(fs.Updates.Ostree.FilePath(tag, updateName, objRel)), 0o755); err != nil {
		return "", err
	}
	if err := fs.Updates.Ostree.WriteFile(tag, updateName, objRel, string(content)); err != nil {
		return "", err
	}
	refRel := filepath.Join("refs", "heads", "perf-test")
	if err := os.MkdirAll(filepath.Dir(fs.Updates.Ostree.FilePath(tag, updateName, refRel)), 0o755); err != nil {
		return "", err
	}
	if err := fs.Updates.Ostree.WriteFile(tag, updateName, refRel, hexHash+"\n"); err != nil {
		return "", err
	}
	return hexHash, nil
}

func targetsJSON(targetName, ostreeHash string) (string, error) {
	type targetEntry struct {
		Length int64             `json:"length"`
		Hashes map[string]string `json:"hashes"`
	}
	type signed struct {
		Type        string                 `json:"_type"`
		SpecVersion string                 `json:"spec_version"`
		Version     int                    `json:"version"`
		Expires     string                 `json:"expires"`
		Targets     map[string]targetEntry `json:"targets"`
	}
	doc := struct {
		Signed     signed `json:"signed"`
		Signatures []any  `json:"signatures"`
	}{
		Signed: signed{
			Type:        "Targets",
			SpecVersion: "1.0",
			Version:     1,
			Expires:     time.Now().UTC().AddDate(0, 6, 0).Format(time.RFC3339),
			Targets: map[string]targetEntry{
				targetName: {
					Length: 256 * 1024,
					Hashes: map[string]string{"sha256": ostreeHash},
				},
			},
		},
		Signatures: []any{},
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	return string(b), err
}

func snapshotJSON(expires string) string {
	return fmt.Sprintf(`{
  "signed": {
    "_type": "Snapshot",
    "spec_version": "1.0",
    "version": 1,
    "expires": %q,
    "meta": {"targets.json": {"version": 1}}
  },
  "signatures": []
}`, expires)
}

func timestampJSON(expires string) string {
	return fmt.Sprintf(`{
  "signed": {
    "_type": "Timestamp",
    "spec_version": "1.0",
    "version": 1,
    "expires": %q,
    "meta": {"snapshot.json": {"version": 1}}
  },
  "signatures": []
}`, expires)
}

func rootJSON(expires string) string {
	return fmt.Sprintf(`{
  "signed": {
    "_type": "Root",
    "spec_version": "1.0",
    "version": 1,
    "expires": %q,
    "consistent_snapshot": false,
    "keys": {},
    "roles": {
      "root":      {"keyids": [], "threshold": 1},
      "targets":   {"keyids": [], "threshold": 1},
      "snapshot":  {"keyids": [], "threshold": 1},
      "timestamp": {"keyids": [], "threshold": 1}
    }
  },
  "signatures": []
}`, expires)
}
