// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"fmt"
	"testing"

	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/api"
	"github.com/stretchr/testify/require"
)

// TestSeedDeviceUpdateHistory verifies that update history is seeded for
// only a subset of devices, and that seeded devices get 10-15 entries.
// Uses enough devices that both outcomes (some, none) are virtually certain
// to occur, given the ~1/3 selection chance in seedDeviceUpdateHistory.
func TestSeedDeviceUpdateHistory(t *testing.T) {
	datadir := t.TempDir()
	const numDevices = 30

	require.NoError(t, seedDevices(datadir, numDevices))

	fs, err := storage.NewFs(datadir)
	require.NoError(t, err)
	db, err := storage.NewDb(fs.Config.DbFile())
	require.NoError(t, err)
	ap, err := api.NewStorage(db, fs)
	require.NoError(t, err)

	withHistory, withoutHistory := 0, 0
	for i := 1; i <= numDevices; i++ {
		uuid := fmt.Sprintf("seed-device-%05d", i)
		apiDevice, err := ap.DeviceGet(uuid)
		require.NoError(t, err)
		require.NotNil(t, apiDevice)

		updateIds, err := apiDevice.Updates()
		require.NoError(t, err)

		switch {
		case len(updateIds) == 0:
			withoutHistory++
		default:
			withHistory++
			require.GreaterOrEqual(t, len(updateIds), 10, "seeded history must have >= 10 entries")
			require.LessOrEqual(t, len(updateIds), 15, "seeded history must have <= 15 entries")
		}
	}

	require.Greater(t, withHistory, 0, "expected at least one device with seeded update history")
	require.Greater(t, withoutHistory, 0, "expected at least one device without seeded update history")
}
