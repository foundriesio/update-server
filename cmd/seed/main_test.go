// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"testing"

	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/gateway"
	"github.com/stretchr/testify/require"
)

func TestSeed(t *testing.T) {
	datadir := t.TempDir()

	err := seedDevices(datadir, 3)
	require.NoError(t, err)

	// Verify the first device is retrievable via gateway DeviceGet.
	fs, err := storage.NewFs(datadir)
	require.NoError(t, err)
	db, err := storage.NewDb(fs.Config.DbFile())
	require.NoError(t, err)
	gw, err := gateway.NewStorage(db, fs)
	require.NoError(t, err)

	d, err := gw.DeviceGet("seed-device-00001")
	require.NoError(t, err)
	require.NotNil(t, d, "expected seed-device-00001 to be present in the DB")
}
