// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/api"
	"github.com/foundriesio/update-server/storage/gateway"
)

// seedDeviceConfigHistory saves a few device config revisions so the config
// history page has more than a single entry to render. Config content is
// stored as JSON-encoded {filename: ConfigFile}, keyed by the sota override
// filename devices actually read (storage.ConfigSotaOverride).
func seedDeviceConfigHistory(ap *api.Storage, uuid string) error {
	revisions := []struct{ pacman, reason string }{
		{"ostree", "initial config"},
		{"ostree+compose_apps", "enable compose apps"},
		{"ostree+compose_apps,restorable", "enable restorable compose apps"},
	}
	for _, rev := range revisions {
		tomlConfig := fmt.Sprintf(`[device]
  uuid = "%s"
  tag = "main"

[pacman]
  type = "%s"
`, uuid, rev.pacman)
		files := map[string]api.ConfigFile{
			storage.ConfigSotaOverride: {Value: tomlConfig},
		}
		content, err := json.Marshal(files)
		if err != nil {
			return fmt.Errorf("marshal device config for %s: %w", uuid, err)
		}
		if err := ap.SaveDeviceConfig(uuid, string(content), "noauth-fake-user", rev.reason); err != nil {
			return fmt.Errorf("SaveDeviceConfig(%s): %w", uuid, err)
		}
	}
	return nil
}

// seedAppsOverride writes a device config revision that overrides
// pacman.compose_apps to a single app, so the device Apps page's "override"
// branch (a device-selected subset of the target's apps, not the full list)
// has a seeded example.
func seedAppsOverride(ap *api.Storage, uuid, apps string) error {
	tomlConfig := fmt.Sprintf(`[pacman]
  type = "ostree+compose_apps"
  compose_apps = "%s"
`, apps)
	files := map[string]api.ConfigFile{
		storage.ConfigSotaOverride: {Value: tomlConfig},
	}
	content, err := json.Marshal(files)
	if err != nil {
		return fmt.Errorf("marshal apps override for %s: %w", uuid, err)
	}
	return ap.SaveDeviceConfig(uuid, string(content), "noauth-fake-user", "seed: override compose apps")
}

// seedDeviceAppsStates saves a fake apps-states snapshot for the device's
// "Apps States" page.
func seedDeviceAppsStates(d *gateway.Device, i int) error {
	content := fmt.Sprintf(`{
  "deviceTime": "2026-07-23T12:00:00Z",
  "ostree": "%064x",
  "apps": {
    "shellhttpd": {
      "uri": "hub.foundries.io/local-factory/shellhttpd@sha256:%064x",
      "state": "running",
      "services": [
        {"name": "shellhttpd", "hash": "%064x", "health": "healthy", "image": "shellhttpd:latest", "state": "running", "status": "Up 2 hours"}
      ]
    },
    "nginx": {
      "uri": "hub.foundries.io/local-factory/nginx@sha256:%064x",
      "state": "running",
      "services": [
        {"name": "nginx", "hash": "%064x", "health": "healthy", "image": "nginx:latest", "state": "running", "status": "Up 2 hours"}
      ]
    }
  }
}`, i*0xdeadbeef, i*31, i*37, i*41, i*43)
	return d.SaveAppsStates(content)
}

// seedDeviceAppliedConfigs saves a fake merged applied-config envelope for
// the device's "Applied Configs" page.
func seedDeviceAppliedConfigs(d *gateway.Device) error {
	cfg := gateway.AppliedConfigs{
		Files: map[string]gateway.ConfigFile{
			"z-50-fioctl.toml": {Value: "[pacman]\n  type = \"ostree+compose_apps\"\n"},
		},
		AppliedAt: time.Now().Unix(),
	}
	cfg.AuditTrail[2].CreatedBy = "noauth-fake-user"
	cfg.AuditTrail[2].Reason = "seed"
	return d.SaveAppliedConfigs(cfg)
}

// seedDeviceTests creates a couple of fake fiotest results (one passed, one
// failed) for the device's Tests tab, including one stored artifact.
func seedDeviceTests(d *gateway.Device, targetName string, i int) error {
	tests := []struct {
		name, status, details string
	}{
		{"smoke-test", "PASSED", ""},
		{"integration-test", "FAILED", "assertion failed: expected 200, got 500"},
	}
	for j, tc := range tests {
		testId := fmt.Sprintf("seed-test-%05d-%02d", i, j)
		if err := d.TestCreate(targetName, tc.name, testId); err != nil {
			return fmt.Errorf("TestCreate(%s): %w", testId, err)
		}
		results := []storage.TargetTestResult{
			{Name: "boot-time", Status: "PASSED", Details: "", Metrics: map[string]float64{"seconds": 4.2}},
		}
		if err := d.TestComplete(testId, tc.status, tc.details, results); err != nil {
			return fmt.Errorf("TestComplete(%s): %w", testId, err)
		}
		if j == 0 {
			artifact := []byte("seed test log line 1\nseed test log line 2\n")
			if err := d.TestStoreArtifact(testId, "test.log", bytes.NewReader(artifact)); err != nil {
				return fmt.Errorf("TestStoreArtifact(%s): %w", testId, err)
			}
		}
	}
	return nil
}

// updateHistoryStages is the full success event sequence for one simulated
// update. A failed entry only sends the first 4 stages, the last one marked
// unsuccessful, mirroring how a real failed install would look.
var updateHistoryStages = []string{
	"EcuDownloadStarted",
	"EcuDownloadCompleted",
	"EcuInstallationStarted",
	"EcuInstallationApplied",
	"EcuInstallationCompleted",
}

// seedDeviceUpdateHistory randomly gives about a third of devices a long
// (10-15 entry) update history, so the device's "Update History" page isn't
// always sparse, without making every seeded device look identical. Roughly
// one in five entries is seeded as a failed update for visual variety.
func seedDeviceUpdateHistory(d *gateway.Device, i int) error {
	if rand.Intn(3) != 0 {
		return nil
	}

	count := 10 + rand.Intn(6) // 10-15 entries
	for j := 0; j < count; j++ {
		failed := rand.Intn(5) == 0
		stages := updateHistoryStages
		if failed {
			stages = updateHistoryStages[:4]
		}

		version := fmt.Sprintf("%d", 100+i*20+j)
		targetName := fmt.Sprintf("intel-corei7-64-lmp-%s", version)
		corrId := fmt.Sprintf("seed-hist-%d-%02d", i, j)
		base := time.Now().UTC().AddDate(0, 0, -j)

		events := make([]storage.DeviceUpdateEvent, 0, len(stages))
		for k, stage := range stages {
			success := true
			if failed && k == len(stages)-1 {
				success = false
			}
			events = append(events, storage.DeviceUpdateEvent{
				Id:         fmt.Sprintf("%s-%d", corrId, k),
				DeviceTime: base.Add(time.Duration(k) * time.Second).Format(time.RFC3339),
				Event: storage.DeviceEvent{
					CorrelationId: corrId,
					Ecu:           "seed-ecu",
					Success:       &success,
					TargetName:    targetName,
					Version:       version,
				},
				EventType: storage.DeviceEventType{Id: stage, Version: 1},
			})
		}
		if err := d.ProcessEvents(events); err != nil {
			return fmt.Errorf("ProcessEvents(%s, %s): %w", d.Uuid, corrId, err)
		}
	}
	return nil
}
