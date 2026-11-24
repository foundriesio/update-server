// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigHistory(t *testing.T) {
	fs, err := NewFs(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(fs.Close)
	for _, tc := range []struct {
		name  string
		dir   string
		write func(string, string, string) error
		read  func(int, bool) ([]*ConfigFileSet, error)
		purge func(int) error
	}{
		{"factory", ConfigsFactoryDir, fs.Configs.WriteFactoryConfig, fs.Configs.ReadFactoryConfigHistory, fs.Configs.PurgeFactoryConfigHistory},
		{"group", filepath.Join(ConfigsGroupDir, "test"),
			func(c, u, r string) error { return fs.Configs.WriteGroupConfig("test", c, u, r) },
			func(n int, files bool) ([]*ConfigFileSet, error) {
				return fs.Configs.ReadGroupConfigHistory("test", n, files)
			},
			func(n int) error { return fs.Configs.PurgeGroupConfigHistory("test", n) }},
		{"device", filepath.Join(ConfigsDeviceDir, "test"),
			func(c, u, r string) error { return fs.Configs.WriteDeviceConfig("test", c, u, r) },
			func(n int, files bool) ([]*ConfigFileSet, error) {
				return fs.Configs.ReadDeviceConfigHistory("test", n, files)
			},
			func(n int) error { return fs.Configs.PurgeDeviceConfigHistory("test", n) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			history, err := tc.read(5, true)
			require.NoError(t, err)
			require.Empty(t, history)
			require.NoError(t, tc.write("old", "alice", "initial"))
			require.NoError(t, tc.write("new", "bob", "change:reason"))
			history, err = tc.read(5, true)
			require.NoError(t, err)
			require.Len(t, history, 2)
			require.Equal(t, "new", history[0].RawFiles)
			require.Equal(t, "bob", history[0].CreatedBy)
			require.Equal(t, "change:reason", history[0].Reason)
			require.Equal(t, "old", history[1].RawFiles)
			require.NoError(t, tc.purge(1))
			oldPath := filepath.Join(fs.Config.ConfigsDir(), tc.dir, fmt.Sprintf("%x", sha256.Sum256([]byte("old"))))
			_, err = os.Stat(oldPath)
			require.ErrorIs(t, err, os.ErrNotExist)
			history, err = tc.read(1, true)
			require.NoError(t, err)
			require.Len(t, history, 1)
			require.Equal(t, "new", history[0].RawFiles)
			history, err = tc.read(5, false)
			require.NoError(t, err)
			require.Len(t, history, 2)
			require.Empty(t, history[0].RawFiles)
		})
	}
	names, err := fs.Configs.ReadGroupNames()
	require.NoError(t, err)
	require.Equal(t, []string{"test"}, names)
	fs.Close()
	require.Error(t, fs.Configs.WriteFactoryConfig("closed", "", ""))
}

func TestConfigsRejectRootEscape(t *testing.T) {
	fs, err := NewFs(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(fs.Close)
	outside := t.TempDir()
	sentinel := filepath.Join(outside, "data")
	require.NoError(t, os.WriteFile(sentinel, []byte("keep"), defaultFileAccess))
	require.NoError(t, os.MkdirAll(filepath.Join(fs.Config.ConfigsDir(), ConfigsGroupDir), defaultDirAccess))
	require.NoError(t, os.Symlink(outside, filepath.Join(fs.Config.ConfigsDir(), ConfigsGroupDir, "escape")))
	for _, name := range []string{"escape", "../../outside"} {
		require.Error(t, fs.Configs.WriteGroupConfig(name, "new", "", ""))
		_, err := fs.Configs.ReadGroupConfigHistory(name, 5, true)
		require.Error(t, err)
		require.Error(t, fs.Configs.PurgeGroupConfigHistory(name, 1))
	}
	require.NoError(t, fs.Configs.WriteFactoryConfig("initial", "", ""))
	journal := filepath.Join(fs.Config.ConfigsDir(), ConfigsFactoryDir, ConfigsJournalFile)
	require.NoError(t, os.WriteFile(journal, []byte(sentinel+":1234\n"), defaultFileAccess))
	_, err = fs.Configs.ReadFactoryConfigHistory(1, true)
	require.Error(t, err)
	require.NoError(t, os.Remove(journal))
	require.NoError(t, os.Symlink(sentinel, journal))
	require.Error(t, fs.Configs.WriteFactoryConfig("new", "", ""))
	content, err := os.ReadFile(sentinel)
	require.NoError(t, err)
	require.Equal(t, "keep", string(content))
}
