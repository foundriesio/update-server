// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeviceFilesRejectTraversal(t *testing.T) {
	for _, uuid := range []string{"", ".", "..", "../outside", "a/../../outside", `/tmp/device`, `..\outside`, "%2e%2e", "device\n"} {
		t.Run(uuid, func(t *testing.T) {
			root := t.TempDir()
			fs, err := NewFs(root)
			require.NoError(t, err)
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
	for _, uuid := range []string{"foo", "Device_1", "123e4567-e89b-12d3-a456-426614174000"} {
		require.NoError(t, fs.Devices.WriteFile(uuid, "sentinel", "value"))
		content, err := fs.Devices.ReadFile(uuid, "sentinel")
		require.NoError(t, err)
		require.Equal(t, "value", content)
		require.NoError(t, fs.Devices.Delete(uuid))
	}
}
