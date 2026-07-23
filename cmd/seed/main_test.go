// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"testing"

	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/api"
	"github.com/foundriesio/update-server/storage/gateway"
	"github.com/foundriesio/update-server/storage/users"
	"github.com/stretchr/testify/require"
)

func TestSeed(t *testing.T) {
	datadir := t.TempDir()

	err := seedDevices(datadir, 3)
	require.NoError(t, err)

	fs, err := storage.NewFs(datadir)
	require.NoError(t, err)
	require.NoError(t, fs.Auth.InitHmacSecret())
	db, err := storage.NewDb(fs.Config.DbFile())
	require.NoError(t, err)
	gw, err := gateway.NewStorage(db, fs)
	require.NoError(t, err)
	ap, err := api.NewStorage(db, fs)
	require.NoError(t, err)

	d, err := gw.DeviceGet("seed-device-00001")
	require.NoError(t, err)
	require.NotNil(t, d, "expected seed-device-00001 to be present in the DB")

	// Verify device extras: config history, apps-states, applied-configs, tests.
	history, err := ap.ReadDeviceConfigHistory("seed-device-00001", 10, true)
	require.NoError(t, err)
	require.Len(t, history, 3, "expected 3 device config revisions")
	require.Contains(t, history[0].RawFiles, storage.ConfigSotaOverride,
		"device config must be valid JSON keyed by the sota override filename, not raw TOML")

	apiDevice, err := ap.DeviceGet("seed-device-00001")
	require.NoError(t, err)
	require.NotNil(t, apiDevice)

	states, err := apiDevice.AppsStates()
	require.NoError(t, err)
	require.NotEmpty(t, states, "expected apps-states to be seeded")

	applied, err := ap.ReadAppliedConfigs("seed-device-00001")
	require.NoError(t, err)
	require.NotNil(t, applied, "expected applied configs to be seeded")
	require.Greater(t, applied.AppliedAt, int64(0), "expected AppliedAt to be set")

	tests, err := apiDevice.GetTests()
	require.NoError(t, err)
	require.Len(t, tests, 2, "expected 2 seeded test results")

	// Verify global/group configs and users, called the same way main() calls them.
	require.NoError(t, seedGlobalConfigs(ap))
	factoryHistory, err := ap.ReadFactoryConfigHistory(1, false)
	require.NoError(t, err)
	require.Len(t, factoryHistory, 1, "expected factory config to be seeded")

	us, err := users.NewStorage(db, fs)
	require.NoError(t, err)
	require.NoError(t, seedUsers(us))
	u, err := us.Get("seed-operator")
	require.NoError(t, err)
	require.NotNil(t, u, "expected seed-operator user to be seeded")

	// Verify idempotency: seeding again does not error or duplicate config history.
	require.NoError(t, seedDevices(datadir, 3))
	history, err = ap.ReadDeviceConfigHistory("seed-device-00001", 10, false)
	require.NoError(t, err)
	require.Len(t, history, 3, "re-seeding must not duplicate device config history")
}
