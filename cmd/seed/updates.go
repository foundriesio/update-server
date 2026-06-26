// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/api"
)

// updateRegistered reports whether (tag, name) is already in the updates table.
func updateRegistered(apiStorage *api.Storage, tag, name string) (bool, error) {
	existing, err := apiStorage.ListUpdates(tag)
	if err != nil {
		return false, err
	}
	for _, u := range existing[tag] {
		if u.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// fakeHex returns a deterministic lowercase hex string of the requested byte
// length based on the seed index i.  It is not cryptographic — it only needs
// to look plausible in the UI.
func fakeHex(i, length int) string {
	result := make([]byte, length)
	for j := 0; j < length; j++ {
		result[j] = byte(((i+1)*31 + j*17) & 0xff)
	}
	return fmt.Sprintf("%x", result)
}

// targetsJSON builds a structurally-valid TUF targets.json body.
func targetsJSON(i int, name, expires string) (string, error) {
	sha256 := fakeHex(i, 32)      // 64 hex chars
	sha512 := fakeHex(i+100, 64)  // 128 hex chars
	appSha1 := fakeHex(i+200, 32) // shellhttpd
	appSha2 := fakeHex(i+300, 32) // nginx
	keyid := fakeHex(i+400, 32)   // 64 hex chars
	sig := fakeHex(i+500, 64)     // 128 hex chars

	now := time.Now().UTC().Format(time.RFC3339)

	type dockerApp struct {
		URI string `json:"uri"`
	}
	type custom struct {
		HardwareIDs       []string             `json:"hardwareIds"`
		Tags              []string             `json:"tags"`
		TargetFormat      string               `json:"targetFormat"`
		Version           string               `json:"version"`
		Name              string               `json:"name"`
		URI               string               `json:"uri"`
		CreatedAt         string               `json:"createdAt"`
		UpdatedAt         string               `json:"updatedAt"`
		Arch              string               `json:"arch"`
		DockerComposeApps map[string]dockerApp `json:"docker_compose_apps"`
	}
	type targetEntry struct {
		Length int               `json:"length"`
		Hashes map[string]string `json:"hashes"`
		Custom custom            `json:"custom"`
	}
	type signed struct {
		Type        string                 `json:"_type"`
		SpecVersion string                 `json:"spec_version"`
		Version     int                    `json:"version"`
		Expires     string                 `json:"expires"`
		Targets     map[string]targetEntry `json:"targets"`
	}
	type signature struct {
		KeyID  string `json:"keyid"`
		Method string `json:"method"`
		Sig    string `json:"sig"`
	}
	type tufTargets struct {
		Signed     signed      `json:"signed"`
		Signatures []signature `json:"signatures"`
	}

	targetName := fmt.Sprintf("intel-corei7-64-lmp-%s", name)

	doc := tufTargets{
		Signed: signed{
			Type:        "Targets",
			SpecVersion: "1.0",
			Version:     148 + i,
			Expires:     expires,
			Targets: map[string]targetEntry{
				targetName: {
					Length: 0,
					Hashes: map[string]string{
						"sha256": sha256,
						"sha512": sha512,
					},
					Custom: custom{
						HardwareIDs:  []string{"intel-corei7-64"},
						Tags:         []string{"main"},
						TargetFormat: "OSTREE",
						Version:      name,
						Name:         "intel-corei7-64-lmp",
						URI:          "",
						CreatedAt:    now,
						UpdatedAt:    now,
						Arch:         "amd64",
						DockerComposeApps: map[string]dockerApp{
							"shellhttpd": {URI: fmt.Sprintf("hub.foundries.io/local-factory/shellhttpd@sha256:%s", appSha1)},
							"nginx":      {URI: fmt.Sprintf("hub.foundries.io/local-factory/nginx@sha256:%s", appSha2)},
						},
					},
				},
			},
		},
		Signatures: []signature{
			{KeyID: keyid, Method: "eddsa", Sig: sig},
		},
	}

	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func snapshotJSON(i int, expires string) string {
	keyid := fakeHex(i+600, 32)
	sig := fakeHex(i+700, 64)
	return fmt.Sprintf(`{
  "signed": {
    "_type": "Snapshot",
    "spec_version": "1.0",
    "version": %d,
    "expires": "%s",
    "meta": {
      "targets.json": {"version": %d, "length": 512, "hashes": {"sha256": "%s"}}
    }
  },
  "signatures": [{"keyid": "%s", "method": "eddsa", "sig": "%s"}]
}`, 148+i, expires, 148+i, fakeHex(i+800, 32), keyid, sig)
}

func timestampJSON(i int, expires string) string {
	keyid := fakeHex(i+900, 32)
	sig := fakeHex(i+1000, 64)
	return fmt.Sprintf(`{
  "signed": {
    "_type": "Timestamp",
    "spec_version": "1.0",
    "version": %d,
    "expires": "%s",
    "meta": {
      "snapshot.json": {"version": %d, "length": 256, "hashes": {"sha256": "%s"}}
    }
  },
  "signatures": [{"keyid": "%s", "method": "eddsa", "sig": "%s"}]
}`, 148+i, expires, 148+i, fakeHex(i+1100, 32), keyid, sig)
}

func rootJSON(i int, expires string) string {
	keyid := fakeHex(i+1200, 32)
	sig := fakeHex(i+1300, 64)
	pubkeyVal := fakeHex(i+1400, 32)
	return fmt.Sprintf(`{
  "signed": {
    "_type": "Root",
    "spec_version": "1.0",
    "version": 1,
    "expires": "%s",
    "consistent_snapshot": false,
    "keys": {
      "%s": {
        "keytype": "ed25519",
        "scheme": "ed25519",
        "keyid_hash_algorithms": ["sha256","sha512"],
        "keyval": {"public": "%s"}
      }
    },
    "roles": {
      "root":      {"keyids": ["%s"], "threshold": 1},
      "targets":   {"keyids": ["%s"], "threshold": 1},
      "snapshot":  {"keyids": ["%s"], "threshold": 1},
      "timestamp": {"keyids": ["%s"], "threshold": 1}
    }
  },
  "signatures": [{"keyid": "%s", "method": "eddsa", "sig": "%s"}]
}`, expires, keyid, pubkeyVal, keyid, keyid, keyid, keyid, keyid, sig)
}

// seedUpdates creates `count` fake update entries (TUF metadata + token dirs +
// one rollout each) under <datadir>/updates/main/.
func seedUpdates(fs *storage.FsHandle, apiStorage *api.Storage, count int) error {
	const tag = "main"
	const baseVersion = 148

	expires := time.Now().AddDate(0, 6, 0).UTC().Format(time.RFC3339)

	created := 0
	skipped := 0

	for i := 0; i < count; i++ {
		name := fmt.Sprintf("%d", baseVersion+i)

		// --- TUF files ---

		targets, err := targetsJSON(i, name, expires)
		if err != nil {
			return fmt.Errorf("build targets.json for %s/%s: %w", tag, name, err)
		}
		if err := fs.Updates.Tuf.WriteFile(tag, name, "targets.json", targets); err != nil {
			return fmt.Errorf("write targets.json for %s/%s: %w", tag, name, err)
		}
		if err := fs.Updates.Tuf.WriteFile(tag, name, "snapshot.json", snapshotJSON(i, expires)); err != nil {
			return fmt.Errorf("write snapshot.json for %s/%s: %w", tag, name, err)
		}
		if err := fs.Updates.Tuf.WriteFile(tag, name, "timestamp.json", timestampJSON(i, expires)); err != nil {
			return fmt.Errorf("write timestamp.json for %s/%s: %w", tag, name, err)
		}
		if err := fs.Updates.Tuf.WriteFile(tag, name, "1.root.json", rootJSON(i, expires)); err != nil {
			return fmt.Errorf("write 1.root.json for %s/%s: %w", tag, name, err)
		}

		// Register the update in the DB so it shows up in ListUpdates.
		// ponytail: skip if already present to stay idempotent.
		registered, err := updateRegistered(apiStorage, tag, name)
		if err != nil {
			return fmt.Errorf("list updates for %s/%s: %w", tag, name, err)
		}
		if !registered {
			if err := apiStorage.InsertUpdate(tag, name, "seed"); err != nil {
				return fmt.Errorf("insert update %s/%s: %w", tag, name, err)
			}
		}

		// --- Token files for ostree_repo and apps ---

		const ostreeConfig = "[core]\nrepo_version=1\nmode=archive-z2\n"
		if err := fs.Updates.Ostree.WriteFile(tag, name, "config", ostreeConfig); err != nil {
			return fmt.Errorf("write ostree config for %s/%s: %w", tag, name, err)
		}

		const ociLayout = `{"imageLayoutVersion":"1.0.0"}`
		if err := fs.Updates.Apps.WriteFile(tag, name, "oci-layout", ociLayout); err != nil {
			return fmt.Errorf("write oci-layout for %s/%s: %w", tag, name, err)
		}
		const indexJSON = `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[]}`
		if err := fs.Updates.Apps.WriteFile(tag, name, "index.json", indexJSON); err != nil {
			return fmt.Errorf("write index.json for %s/%s: %w", tag, name, err)
		}

		// --- Rollout (idempotent) ---

		const rolloutName = "seed-rollout"
		existing, err := fs.Updates.Rollouts.ListFiles(tag, name)
		if err != nil {
			return fmt.Errorf("list rollouts for %s/%s: %w", tag, name, err)
		}
		rolloutExists := false
		for _, f := range existing {
			if f == rolloutName {
				rolloutExists = true
				break
			}
		}
		if rolloutExists {
			log.Printf("skip  rollout %s/%s/%s (already exists)", tag, name, rolloutName)
			skipped++
		} else {
			if err := apiStorage.CreateRollout(tag, name, rolloutName, api.Rollout{
				Groups: []string{"alpha"},
			}); err != nil {
				return fmt.Errorf("CreateRollout for %s/%s: %w", tag, name, err)
			}
			log.Printf("create update %s/%s + rollout %s", tag, name, rolloutName)
			created++
		}
	}

	fmt.Printf("updates seed complete: %d created, %d skipped (total requested: %d)\n", created, skipped, count)
	return nil
}
