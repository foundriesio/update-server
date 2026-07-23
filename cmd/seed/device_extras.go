// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
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
