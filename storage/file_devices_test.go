// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDeviceFilesRejectTraversal(t *testing.T) {
	for _, uuid := range []string{"", ".", "..", "../outside", "a/../../outside", `/tmp/device`, `..\outside`, "%2e%2e", "device\n"} {
		t.Run(uuid, func(t *testing.T) {
			root := t.TempDir()
			fs, err := NewFs(root)
			require.NoError(t, err)
			t.Cleanup(fs.Close)
			sentinel := filepath.Join(root, "outside", "sentinel")
			require.NoError(t, os.MkdirAll(filepath.Dir(sentinel), 0o750))
			require.NoError(t, os.WriteFile(sentinel, []byte("keep"), 0o600))
			require.Error(t, fs.Devices.Delete(uuid))
			require.Error(t, fs.Devices.WriteFile(uuid, "sentinel", "changed"))
			_, err = fs.Devices.ReadFile(uuid, "sentinel")
			require.Error(t, err)
			_, err = fs.Devices.ReadFileStream(uuid, "sentinel")
			require.Error(t, err)
			_, err = fs.Devices.ListFiles(uuid, "", false)
			require.Error(t, err)
			content, err := os.ReadFile(sentinel)
			require.NoError(t, err)
			require.Equal(t, "keep", string(content))
		})
	}
}

func TestDeviceUUIDValidIdentifiers(t *testing.T) {
	fs, err := NewFs(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(fs.Close)
	for _, uuid := range []string{"foo", "Device_1", "123e4567-e89b-12d3-a456-426614174000"} {
		require.NoError(t, fs.Devices.WriteFile(uuid, "sentinel", "value"))
		content, err := fs.Devices.ReadFile(uuid, "sentinel")
		require.NoError(t, err)
		require.Equal(t, "value", content)
		require.NoError(t, fs.Devices.Delete(uuid))
	}
}

func TestDeviceRootOperations(t *testing.T) {
	fs, err := NewFs(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(fs.Close)
	content, err := fs.Devices.ReadFile("device", "missing")
	require.NoError(t, err)
	require.Empty(t, content)
	names, err := fs.Devices.ListFiles("device", "", false)
	require.NoError(t, err)
	require.Empty(t, names)
	_, err = fs.Devices.ReadFileStream("device", "missing")
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, fs.Devices.Delete("device"))
	require.NoError(t, fs.Devices.WriteFile("device", "info", "old"))
	require.NoError(t, fs.Devices.WriteFile("device", "info", "new"))
	require.NoError(t, fs.Devices.AppendFile("device", "info", " appended"))
	content, err = fs.Devices.ReadFile("device", "info")
	require.NoError(t, err)
	require.Equal(t, "new appended", content)
	require.NoError(t, fs.Devices.WriteFileStream("device", "artifact", strings.NewReader("artifact data")))
	stream, err := fs.Devices.ReadFileStream("device", "artifact")
	require.NoError(t, err)
	data, err := io.ReadAll(stream)
	require.NoError(t, err)
	require.NoError(t, stream.Close())
	require.Equal(t, "artifact data", string(data))
	for i, name := range []string{"events-new", "events-old"} {
		require.NoError(t, fs.Devices.WriteFile("device", name, name))
		at := time.Unix(int64(2-i), 0)
		require.NoError(t, os.Chtimes(filepath.Join(fs.Config.DevicesDir(), "device", name), at, at))
	}
	require.NoError(t, fs.Devices.WriteFile("device", "events-hidden"+partialFileSuffix, "partial"))
	names, err = fs.Devices.ListFiles("device", "events-", true)
	require.NoError(t, err)
	require.Equal(t, []string{"events-old", "events-new"}, names)
	require.NoError(t, fs.Devices.RolloverFiles("device", "events-", 1))
	names, err = fs.Devices.ListFiles("device", "events-", true)
	require.NoError(t, err)
	require.Equal(t, []string{"events-new"}, names)
	require.NoError(t, fs.Devices.Delete("device"))
	require.NoError(t, fs.Devices.Delete("device"))
	fs.Close()
	require.Error(t, fs.Devices.WriteFile("device", "info", "closed"))
}

func TestDeviceFilesRejectSymlinkEscape(t *testing.T) {
	fs, err := NewFs(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(fs.Close)
	outside := t.TempDir()
	sentinel := filepath.Join(outside, "sentinel")
	require.NoError(t, os.WriteFile(sentinel, []byte("keep"), defaultFileAccess))
	require.NoError(t, os.Symlink(outside, filepath.Join(fs.Config.DevicesDir(), "escape")))
	require.NoError(t, fs.Devices.WriteFile("device", "info", "value"))
	require.NoError(t, os.Symlink(sentinel, filepath.Join(fs.Config.DevicesDir(), "device", "link")))
	for _, tc := range [][2]string{{"escape", "sentinel"}, {"device", "link"}, {"device", "../escape/sentinel"}} {
		_, err := fs.Devices.ReadFile(tc[0], tc[1])
		require.Error(t, err)
		_, err = fs.Devices.ReadFileStream(tc[0], tc[1])
		require.Error(t, err)
		require.Error(t, fs.Devices.AppendFile(tc[0], tc[1], "append"))
		require.Error(t, fs.Devices.WriteFileStream(tc[0], tc[1], strings.NewReader("replace")))
	}
	require.Error(t, fs.Devices.WriteFile("escape", "sentinel", "replace"))
	require.Error(t, fs.Devices.RolloverFiles("escape", "", 0))
	_, err = fs.Devices.ListFiles("escape", "", false)
	require.Error(t, err)
	require.NoError(t, os.Symlink(sentinel, filepath.Join(fs.Config.DevicesDir(), "device", "info"+partialFileSuffix)))
	require.Error(t, fs.Devices.WriteFile("device", "info", "replace"))
	require.NoError(t, fs.Devices.Delete("escape"))
	data, err := os.ReadFile(sentinel)
	require.NoError(t, err)
	require.Equal(t, "keep", string(data))
}
