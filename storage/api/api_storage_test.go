// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/gateway"
	storageTesting "github.com/foundriesio/update-server/storage/testing"
)

func TestStorage(t *testing.T) {
	tmpdir := t.TempDir()
	dbFile := filepath.Join(tmpdir, "sql.db")
	db, err := storage.NewDb(dbFile)
	require.Nil(t, err)
	fs, err := storage.NewFs(tmpdir)
	require.Nil(t, err)

	s, err := NewStorage(db, fs)
	require.Nil(t, err)

	dg, err := gateway.NewStorage(db, fs)
	require.Nil(t, err)

	// Test 404 type operation
	d, err := s.DeviceGet("does not exist")
	require.Nil(t, err)
	require.Nil(t, d)

	// Test we can list when there are no devices
	opts := DeviceListOpts{}
	devices, count, err := s.DevicesList(opts)
	require.Nil(t, err)
	require.Equal(t, 0, len(devices))
	require.Equal(t, 0, count)

	// Create two devices to list/get on
	d2, err := dg.DeviceCreate("uuid-1", "pubkey-value-1")
	require.Nil(t, err)
	require.Nil(t, d2.PutFile(storage.AktomlFile, "aktoml content"))
	require.Nil(t, d2.CheckIn("target", "tag", "hash", ""))
	time.Sleep(time.Second)
	_, err = dg.DeviceCreate("uuid-2", "pubkey-value-2")
	require.Nil(t, err)

	uuids, err := s.SetUpdateName("tag", "update42", []string{"uuid-1", "uuid-2"}, nil)
	require.Nil(t, err)
	require.Equal(t, 1, len(uuids))
	assert.Equal(t, "uuid-1", uuids[0])

	opts.Limit = 2
	opts.OrderBy = OrderByDeviceCreatedAsc
	devices, count, err = s.DevicesList(opts)
	require.Nil(t, err)
	require.Equal(t, 2, len(devices))
	require.Equal(t, 2, count)
	assert.Equal(t, "uuid-1", devices[0].Uuid)
	assert.Equal(t, "uuid-2", devices[1].Uuid)

	opts.OrderBy = OrderByDeviceCreatedDsc
	devices, count, err = s.DevicesList(opts)
	require.Nil(t, err)
	require.Equal(t, 2, len(devices))
	require.Equal(t, 2, count)
	assert.Equal(t, "uuid-2", devices[0].Uuid)

	d, err = s.DeviceGet("uuid-1")
	require.Nil(t, err)
	assert.Equal(t, "hash", d.OstreeHash)
	assert.Equal(t, "tag", d.Tag)
	assert.Equal(t, "pubkey-value-1", d.PubKey)
	assert.Equal(t, "update42", d.UpdateName)
	assert.Equal(t, "aktoml content", d.Aktoml)
}

func TestDeviceDelete(t *testing.T) {
	tmpdir := t.TempDir()
	dbFile := filepath.Join(tmpdir, "sql.db")
	db, err := storage.NewDb(dbFile)
	require.Nil(t, err)
	fs, err := storage.NewFs(tmpdir)
	require.Nil(t, err)

	s, err := NewStorage(db, fs)
	require.Nil(t, err)

	dg, err := gateway.NewStorage(db, fs)
	require.Nil(t, err)

	// Create a device
	_, err = dg.DeviceCreate("uuid-del", "pubkey-del")
	require.Nil(t, err)

	// Verify it exists
	d, err := s.DeviceGet("uuid-del")
	require.Nil(t, err)
	require.NotNil(t, d)

	// Delete it
	require.Nil(t, d.Delete())

	// Verify it no longer shows up in Get or List
	d, err = s.DeviceGet("uuid-del")
	require.Nil(t, err)
	require.Nil(t, d, "deleted device should not be returned by DeviceGet")

	devices, count, err := s.DevicesList(DeviceListOpts{Limit: 100})
	require.Nil(t, err)
	assert.Equal(t, 0, count, "deleted device should not appear in DevicesList")
	for _, dev := range devices {
		assert.NotEqual(t, "uuid-del", dev.Uuid, "deleted device should not appear in DevicesList")
	}
}

func TestDeviceRestore(t *testing.T) {
	tmpdir := t.TempDir()
	dbFile := filepath.Join(tmpdir, "sql.db")
	db, err := storage.NewDb(dbFile)
	require.Nil(t, err)
	fs, err := storage.NewFs(tmpdir)
	require.Nil(t, err)

	s, err := NewStorage(db, fs)
	require.Nil(t, err)

	dg, err := gateway.NewStorage(db, fs)
	require.Nil(t, err)

	// Removing a device that does not exist reports "not found".
	undenied, err := s.UndenyDevice("no-such-device")
	require.Nil(t, err)
	assert.False(t, undenied, "removing an unknown device from denied list should report false")

	// Create and delete a device.
	_, err = dg.DeviceCreate("uuid-restore", "pubkey-restore")
	require.Nil(t, err)
	d, err := s.DeviceGet("uuid-restore")
	require.Nil(t, err)
	require.NotNil(t, d)
	require.Nil(t, d.Delete())

	// The denied device shows up in the denied list, not the active list.
	uuids, err := s.DeniedDevicesList()
	require.Nil(t, err)
	assert.Equal(t, 1, len(uuids), "deleted device should appear in DeniedDevicesList")
	assert.Equal(t, "uuid-restore", uuids[0])

	devices, count, err := s.DevicesList(DeviceListOpts{Limit: 100})
	require.Nil(t, err)
	assert.Equal(t, 0, count, "deleted device should not appear in DevicesList")
	assert.Equal(t, 0, len(devices))

	// Remove from denied list.
	undenied, err = s.UndenyDevice("uuid-restore")
	require.Nil(t, err)
	assert.True(t, undenied, "removing a denied device should report true")

	// It is back in the active list/get and gone from the denied list.
	d, err = s.DeviceGet("uuid-restore")
	require.Nil(t, err)
	require.NotNil(t, d, "un-denied device should be returned by DeviceGet")

	_, count, err = s.DevicesList(DeviceListOpts{Limit: 100})
	require.Nil(t, err)
	assert.Equal(t, 1, count, "un-denied device should appear in DevicesList")

	uuids, err = s.DeniedDevicesList()
	require.Nil(t, err)
	assert.Equal(t, 0, len(uuids), "un-denied device should not appear in DeniedDevicesList")

	// Calling UndenyDevice on an already-active device returns false (not on denied list).
	undenied, err = s.UndenyDevice("uuid-restore")
	require.Nil(t, err)
	assert.False(t, undenied, "removing an already-active device from denied list should report false")
}

func TestUploadConfigs(t *testing.T) {
	tmpdir := t.TempDir()
	dbFile := filepath.Join(tmpdir, "sql.db")
	db, err := storage.NewDb(dbFile)
	require.Nil(t, err)
	fs, err := storage.NewFs(tmpdir)
	require.Nil(t, err)
	s, err := NewStorage(db, fs)
	require.Nil(t, err)

	createTar := storageTesting.CreateTarBuffer

	t.Run("Successful initial configs upload", func(t *testing.T) {
		validTarFiles := map[string]string{
			"factory/.journal":     "deadbeef:123456\nelvisalive:137137\n",
			"factory/deadbeef":     `{"test":{"Value":"test factory config"}}`,
			"group/beta/.journal":  "killbill:2003\n",
			"group/beta/killbill":  `{"samurai":{"Value":"test group config"}}`,
			"factory/elvisalive":   `{"test":{"Value":"test factory config latest version"}}`,
			"device/uuid/.journal": "",
		}
		r := createTar(t, validTarFiles)
		require.NoError(t, s.UploadConfigs(r))

		history, err := s.fs.Configs.ReadFactoryConfigHistory(5, true)
		require.NoError(t, err)
		require.Equal(t, 2, len(history))
		assert.Equal(t, `{"test":{"Value":"test factory config latest version"}}`, history[0].RawFiles)
		assert.Equal(t, `{"test":{"Value":"test factory config"}}`, history[1].RawFiles)
		history, err = s.fs.Configs.ReadGroupConfigHistory("beta", 5, true)
		require.NoError(t, err)
		require.Equal(t, 1, len(history))
		assert.Equal(t, `{"samurai":{"Value":"test group config"}}`, history[0].RawFiles)
		history, err = s.fs.Configs.ReadDeviceConfigHistory("uuid", 5, true)
		require.NoError(t, err)
		require.Equal(t, 0, len(history))
	})

	t.Run("Successful upload overwrites existing configs", func(t *testing.T) {
		validTarFiles := map[string]string{
			"factory/.journal":     "deadbeef:123456",
			"factory/deadbeef":     `{"test":{"Value":"overwritten"}}`,
			"group/alpha/.journal": "beep:2003\n",
			"group/alpha/beep":     `{"omega":{"Value":"contra spem spero"}}`,
		}
		r := createTar(t, validTarFiles)
		require.NoError(t, s.UploadConfigs(r))

		history, err := s.fs.Configs.ReadFactoryConfigHistory(5, true)
		require.NoError(t, err)
		require.Equal(t, 1, len(history))
		assert.Equal(t, `{"test":{"Value":"overwritten"}}`, history[0].RawFiles)
		history, err = s.fs.Configs.ReadGroupConfigHistory("beta", 5, true)
		require.NoError(t, err)
		require.Equal(t, 0, len(history))
		history, err = s.fs.Configs.ReadGroupConfigHistory("alpha", 5, true)
		require.NoError(t, err)
		require.Equal(t, 1, len(history))
		assert.Equal(t, `{"omega":{"Value":"contra spem spero"}}`, history[0].RawFiles)
		history, err = s.fs.Configs.ReadDeviceConfigHistory("uuid", 5, true)
		require.NoError(t, err)
		require.Equal(t, 0, len(history))
	})

	t.Run("Failure on input read error", func(t *testing.T) {
		r, w := io.Pipe()
		fail := errors.New("some error")
		require.NoError(t, w.CloseWithError(fail))
		err := s.UploadConfigs(r)
		require.ErrorIs(t, err, fail)
		require.ErrorContains(t, err, "failed to save")
	})

	t.Run("Failure or corrupted tar file", func(t *testing.T) {
		r := bytes.NewBufferString("bad file")
		err := s.UploadConfigs(r)
		require.ErrorIs(t, err, io.ErrUnexpectedEOF)
		require.ErrorContains(t, err, "failed to unpack")
	})

	t.Run("Failure on empty file name", func(t *testing.T) {
		r := createTar(t, map[string]string{"": "some file"})
		err := s.UploadConfigs(r)
		require.ErrorContains(t, err, "failed to unpack")
		require.ErrorContains(t, err, "empty")
	})

	t.Run("Failure on escaping file path", func(t *testing.T) {
		for _, file := range []string{"..", "../outside", "something/../../fancy"} {
			t.Run(file, func(t *testing.T) {
				r := createTar(t, map[string]string{file: "some file"})
				err := s.UploadConfigs(r)
				require.ErrorContains(t, err, "failed to unpack")
				require.ErrorContains(t, err, "escape")
			})
		}
	})
}
