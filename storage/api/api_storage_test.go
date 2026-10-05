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
	require.NoError(t, err)
	fs, err := storage.NewFs(tmpdir)
	require.NoError(t, err)

	s, err := NewStorage(db, fs)
	require.NoError(t, err)

	dg, err := gateway.NewStorage(db, fs)
	require.NoError(t, err)

	// Test 404 type operation
	d, err := s.DeviceGet("does not exist")
	require.NoError(t, err)
	require.Nil(t, d)

	// Test we can list when there are no devices
	opts := DeviceListOpts{}
	devices, count, err := s.DevicesList(opts)
	require.NoError(t, err)
	require.Empty(t, devices)
	require.Equal(t, 0, count)

	// Create two devices to list/get on
	d2, err := dg.DeviceCreate("uuid-1", "cert-value-1")
	require.NoError(t, err)
	require.NoError(t, d2.PutFile(storage.AktomlFile, "aktoml content"))
	require.NoError(t, d2.CheckIn("target", "tag", "hash", ""))
	time.Sleep(time.Second)
	_, err = dg.DeviceCreate("uuid-2", "cert-value-2")
	require.NoError(t, err)

	uuids, err := s.SetUpdateName("tag", "update42", []string{"uuid-1", "uuid-2"}, nil)
	require.NoError(t, err)
	require.Len(t, uuids, 1)
	assert.Equal(t, "uuid-1", uuids[0])

	opts.Limit = 2
	opts.OrderBy = OrderByDeviceCreatedAsc
	devices, count, err = s.DevicesList(opts)
	require.NoError(t, err)
	require.Len(t, devices, 2)
	require.Equal(t, 2, count)
	assert.Equal(t, "uuid-1", devices[0].Uuid)
	assert.Equal(t, "uuid-2", devices[1].Uuid)

	opts.OrderBy = OrderByDeviceCreatedDsc
	devices, count, err = s.DevicesList(opts)
	require.NoError(t, err)
	require.Len(t, devices, 2)
	require.Equal(t, 2, count)
	assert.Equal(t, "uuid-2", devices[0].Uuid)

	d, err = s.DeviceGet("uuid-1")
	require.NoError(t, err)
	assert.Equal(t, "hash", d.OstreeHash)
	assert.Equal(t, "tag", d.Tag)
	assert.Equal(t, "cert-value-1", d.Cert)
	assert.Equal(t, "update42", d.UpdateName)
	assert.Equal(t, "aktoml content", d.Aktoml)
}

func TestDeviceDelete(t *testing.T) {
	tmpdir := t.TempDir()
	dbFile := filepath.Join(tmpdir, "sql.db")
	db, err := storage.NewDb(dbFile)
	require.NoError(t, err)
	fs, err := storage.NewFs(tmpdir)
	require.NoError(t, err)

	s, err := NewStorage(db, fs)
	require.NoError(t, err)

	dg, err := gateway.NewStorage(db, fs)
	require.NoError(t, err)

	// Create a device
	_, err = dg.DeviceCreate("uuid-del", "cert-del")
	require.NoError(t, err)
	name := "device-name"
	group := "device-group"
	require.NoError(t, s.PatchDeviceLabels(map[string]*string{
		"name":  &name,
		"group": &group,
	}, []string{"uuid-del"}))

	// Verify it exists
	d, err := s.DeviceGet("uuid-del")
	require.NoError(t, err)
	require.NotNil(t, d)

	// Delete it
	require.NoError(t, d.Delete())

	stmt, err := db.Prepare("testDeviceLabelsAfterDelete", `
		SELECT json(labels) FROM devices WHERE uuid=?`)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, stmt.Close()) })
	var labelsJSON []byte
	require.NoError(t, stmt.QueryRow("uuid-del").Scan(&labelsJSON))
	assert.JSONEq(t, "{}", string(labelsJSON))

	// Verify it no longer shows up in Get or List
	d, err = s.DeviceGet("uuid-del")
	require.NoError(t, err)
	require.Nil(t, d, "deleted device should not be returned by DeviceGet")

	devices, count, err := s.DevicesList(DeviceListOpts{Limit: 100})
	require.NoError(t, err)
	assert.Equal(t, 0, count, "deleted device should not appear in DevicesList")
	for _, dev := range devices {
		assert.NotEqual(t, "uuid-del", dev.Uuid, "deleted device should not appear in DevicesList")
	}
}

func TestDeviceRestore(t *testing.T) {
	tmpdir := t.TempDir()
	dbFile := filepath.Join(tmpdir, "sql.db")
	db, err := storage.NewDb(dbFile)
	require.NoError(t, err)
	fs, err := storage.NewFs(tmpdir)
	require.NoError(t, err)

	s, err := NewStorage(db, fs)
	require.NoError(t, err)

	dg, err := gateway.NewStorage(db, fs)
	require.NoError(t, err)

	// Removing a device that does not exist reports "not found".
	undenied, err := s.UndenyDevice("no-such-device")
	require.NoError(t, err)
	assert.False(t, undenied, "removing an unknown device from denied list should report false")

	// Create and delete a device.
	_, err = dg.DeviceCreate("uuid-restore", "cert-restore")
	require.NoError(t, err)
	d, err := s.DeviceGet("uuid-restore")
	require.NoError(t, err)
	require.NotNil(t, d)
	require.NoError(t, d.Delete())

	// The denied device shows up in the denied list, not the active list.
	uuids, err := s.DeniedDevicesList()
	require.NoError(t, err)
	assert.Len(t, uuids, 1, "deleted device should appear in DeniedDevicesList")
	assert.Equal(t, "uuid-restore", uuids[0])

	devices, count, err := s.DevicesList(DeviceListOpts{Limit: 100})
	require.NoError(t, err)
	assert.Equal(t, 0, count, "deleted device should not appear in DevicesList")
	assert.Empty(t, devices)

	// Remove from denied list.
	undenied, err = s.UndenyDevice("uuid-restore")
	require.NoError(t, err)
	assert.True(t, undenied, "removing a denied device should report true")

	// It is back in the active list/get and gone from the denied list.
	d, err = s.DeviceGet("uuid-restore")
	require.NoError(t, err)
	require.NotNil(t, d, "un-denied device should be returned by DeviceGet")

	_, count, err = s.DevicesList(DeviceListOpts{Limit: 100})
	require.NoError(t, err)
	assert.Equal(t, 1, count, "un-denied device should appear in DevicesList")

	uuids, err = s.DeniedDevicesList()
	require.NoError(t, err)
	assert.Empty(t, uuids, "un-denied device should not appear in DeniedDevicesList")

	// Calling UndenyDevice on an already-active device returns false (not on denied list).
	undenied, err = s.UndenyDevice("uuid-restore")
	require.NoError(t, err)
	assert.False(t, undenied, "removing an already-active device from denied list should report false")
}

func TestUploadConfigs(t *testing.T) {
	tmpdir := t.TempDir()
	dbFile := filepath.Join(tmpdir, "sql.db")
	db, err := storage.NewDb(dbFile)
	require.NoError(t, err)
	fs, err := storage.NewFs(tmpdir)
	require.NoError(t, err)
	s, err := NewStorage(db, fs)
	require.NoError(t, err)

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
		require.Len(t, history, 2)
		assert.JSONEq(t, `{"test":{"Value":"test factory config latest version"}}`, history[0].RawFiles)
		assert.JSONEq(t, `{"test":{"Value":"test factory config"}}`, history[1].RawFiles)
		history, err = s.fs.Configs.ReadGroupConfigHistory("beta", 5, true)
		require.NoError(t, err)
		require.Len(t, history, 1)
		assert.JSONEq(t, `{"samurai":{"Value":"test group config"}}`, history[0].RawFiles)
		history, err = s.fs.Configs.ReadDeviceConfigHistory("uuid", 5, true)
		require.NoError(t, err)
		require.Empty(t, history)
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
		require.Len(t, history, 1)
		assert.JSONEq(t, `{"test":{"Value":"overwritten"}}`, history[0].RawFiles)
		history, err = s.fs.Configs.ReadGroupConfigHistory("beta", 5, true)
		require.NoError(t, err)
		require.Empty(t, history)
		history, err = s.fs.Configs.ReadGroupConfigHistory("alpha", 5, true)
		require.NoError(t, err)
		require.Len(t, history, 1)
		assert.JSONEq(t, `{"omega":{"Value":"contra spem spero"}}`, history[0].RawFiles)
		history, err = s.fs.Configs.ReadDeviceConfigHistory("uuid", 5, true)
		require.NoError(t, err)
		require.Empty(t, history)
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
