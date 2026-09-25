// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

// Factory config seeding: without this, GetConfigs() returns timestamp=0
// for every device (no factory/group/device config ever written), so
// configGet (server/gateway/handlers_config.go) returns 204 before it ever
// reaches the If-Modified-Since check — the device:config-warm scene has
// nothing to compare a warm cache against. Always on (unlike --seed-update):
// a real fleet always has a factory config, so this makes device:config
// traffic more representative for every scene, not just the warm/cold pair.

package main

import (
	"fmt"

	"github.com/foundriesio/update-server/storage"
)

// seedConfig writes a minimal factory config so every device's GetConfigs()
// call returns a non-zero timestamp. Factory-level is sufficient: configGet
// merges factory+group+device configs and takes the latest CreatedAt across
// all three, so seeding here alone makes the timestamp (and therefore the
// If-Modified-Since path) reachable for every device, without per-device
// writes. The content itself doesn't matter beyond being valid, non-empty
// JSON — devices under test never read Files, only the response headers.
func seedConfig(datadir string) error {
	fs, err := storage.NewFs(datadir)
	if err != nil {
		return fmt.Errorf("open filesystem: %w", err)
	}
	const content = `{"perf-test-fixture.txt":{"Value":"seeded by gen-certs for the perf-test harness"}}`
	if err := fs.Configs.WriteFactoryConfig(content, "perf-test", "seed fixture config"); err != nil {
		return fmt.Errorf("write factory config: %w", err)
	}
	return nil
}
