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

// TestSeedDeviceUpdateHistory verifies that every third device gets 10-15
// update history entries and the remaining devices get none.
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

	require.Equal(t, numDevices/3, withHistory)
	require.Equal(t, numDevices-withHistory, withoutHistory)
}
