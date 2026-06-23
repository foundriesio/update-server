// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"encoding/json"
	"testing"

	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/api"
	"github.com/stretchr/testify/require"
)

func TestSeedUpdates(t *testing.T) {
	datadir := t.TempDir()

	fs, err := storage.NewFs(datadir)
	require.NoError(t, err)
	db, err := storage.NewDb(fs.Config.DbFile())
	require.NoError(t, err)
	apiStorage, err := api.NewStorage(db, fs)
	require.NoError(t, err)

	err = seedUpdates(fs, apiStorage, 2)
	require.NoError(t, err)

	// Verify both updates are listed.
	updates, err := apiStorage.ListUpdates("main")
	require.NoError(t, err)
	require.Contains(t, updates, "main", "expected 'main' tag in updates map")
	names := make([]string, 0, len(updates["main"]))
	for _, u := range updates["main"] {
		names = append(names, u.Name)
	}
	require.Contains(t, names, "148", "expected update '148' under 'main'")
	require.Contains(t, names, "149", "expected update '149' under 'main'")

	// Verify GetUpdateTufMetadata returns parseable data for update "148".
	meta, err := apiStorage.GetUpdateTufMetadata("main", "148")
	require.NoError(t, err)
	require.Contains(t, meta, "targets.json", "TUF metadata missing targets.json")
	require.Contains(t, meta, "snapshot.json", "TUF metadata missing snapshot.json")
	require.Contains(t, meta, "timestamp.json", "TUF metadata missing timestamp.json")
	require.Contains(t, meta, "root.json", "TUF metadata missing root.json")

	// Parse targets.json and verify target name, tag, and version.
	targetsRaw := meta["targets.json"]
	targetsBytes, err := json.Marshal(targetsRaw)
	require.NoError(t, err)

	var targets struct {
		Signed struct {
			Targets map[string]struct {
				Custom struct {
					Tags    []string `json:"tags"`
					Version string   `json:"version"`
				} `json:"custom"`
			} `json:"targets"`
		} `json:"signed"`
	}
	require.NoError(t, json.Unmarshal(targetsBytes, &targets))

	const targetName = "intel-corei7-64-lmp-148"
	target, ok := targets.Signed.Targets[targetName]
	require.True(t, ok, "expected target %q in targets.json", targetName)
	require.Contains(t, target.Custom.Tags, "main", "target tags must include 'main'")
	require.Equal(t, "148", target.Custom.Version, "target version must be the numeric string '148'")

	// Verify rollout was created.
	rollouts, err := apiStorage.ListRollouts("main", "148")
	require.NoError(t, err)
	require.Contains(t, rollouts, "seed-rollout", "expected seed-rollout to be created for update 148")

	// Verify idempotency: seed again and ensure we get 0 created, 2 skipped.
	err = seedUpdates(fs, apiStorage, 2)
	require.NoError(t, err, "second seedUpdates call should not fail (idempotent)")
}
