// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLatestRootMetaName(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected string
	}{
		{
			name:     "single root file",
			files:    []string{"1.root.json"},
			expected: "1.root.json",
		},
		{
			name:     "multiple root files returns highest version",
			files:    []string{"1.root.json", "2.root.json", "3.root.json"},
			expected: "3.root.json",
		},
		{
			name:     "root files out of order",
			files:    []string{"3.root.json", "1.root.json", "2.root.json"},
			expected: "3.root.json",
		},
		{
			name:     "double digit versions",
			files:    []string{"1.root.json", "9.root.json", "10.root.json", "2.root.json"},
			expected: "10.root.json",
		},
		{
			name:     "normal tuf files",
			files:    []string{"timestamp.json", "targets.json", "1.root.json", "snapshot.json"},
			expected: "1.root.json",
		},
		{
			name:     "triple digit versions",
			files:    []string{"1.root.json", "99.root.json", "100.root.json"},
			expected: "100.root.json",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			update := "test-update"
			category := "tuf"
			dir := filepath.Join(tmpDir, update, category)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			for _, f := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, f), []byte("{}"), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			root, err := os.OpenRoot(tmpDir)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = root.Close() })
			h := UpdatesFsHandle{
				osRootHandle: osRootHandle{root: root},
				category:     category,
			}
			got, err := h.LatestRootMetaName(update)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expected {
				t.Errorf("got %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestLatestRootMetaName_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	tag := "test-tag"
	update := "test-update"
	category := "tuf"
	dir := filepath.Join(tmpDir, tag, update, category)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	root, err := os.OpenRoot(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	h := UpdatesFsHandle{
		osRootHandle: osRootHandle{root: root},
		category:     category,
	}
	_, err = h.LatestRootMetaName(update)
	if err == nil {
		t.Fatal("expected error for empty directory, got nil")
	}
}

func TestUpdatesRootOperations(t *testing.T) {
	fs, err := NewFs(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(fs.Close)
	for _, h := range []UpdatesFsHandle{fs.Updates.Apps, fs.Updates.Ostree, fs.Updates.Tuf, fs.Updates.Logs, fs.Updates.Rollouts.UpdatesFsHandle} {
		require.NoError(t, h.WriteFile("v1", "data", "old"))
		require.NoError(t, h.WriteFile("v1", "data", "first\n"))
		require.NoError(t, h.AppendFile("v1", "data", "second\n"))
		content, err := h.ReadFile("v1", "data")
		require.NoError(t, err)
		require.Equal(t, "first\nsecond\n", content)
		var lines []string
		for line, err := range h.TailFileLines("v1", "data", nil) {
			require.NoError(t, err)
			lines = append(lines, line)
		}
		require.Equal(t, []string{"first", "second"}, lines)
	}
	require.NoError(t, fs.Updates.Rollouts.RolloverJournal())
	for _, err := range fs.Updates.Rollouts.ReadJournal() {
		require.NoError(t, err)
		t.Fatal("missing journal yielded a line")
	}
	require.NoError(t, fs.Updates.Rollouts.AppendJournal("entry\n"))
	require.NoError(t, fs.Updates.Rollouts.RolloverJournal())
	var journal []string
	for line, err := range fs.Updates.Rollouts.ReadJournal() {
		require.NoError(t, err)
		journal = append(journal, line)
	}
	require.Equal(t, []string{"entry"}, journal)
	require.NoError(t, fs.Updates.Rollouts.WriteFile("v1", "hidden"+partialFileSuffix, "partial"))
	names, err := fs.Updates.Rollouts.ListFiles("v1")
	require.NoError(t, err)
	require.Equal(t, []string{"data"}, names)
	require.NoError(t, fs.Updates.Delete("v1"))
	require.NoError(t, fs.Updates.Delete("v1"))
	names, err = fs.Updates.Rollouts.ListFiles("v1")
	require.NoError(t, err)
	require.Empty(t, names)
	_, err = fs.Updates.Logs.ReadFile("v1", "data")
	require.ErrorIs(t, err, os.ErrNotExist)
	fs.Close()
	require.Error(t, fs.Updates.Logs.WriteFile("v2", "data", "closed"))
}

func TestUpdatesRejectRootEscape(t *testing.T) {
	fs, err := NewFs(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(fs.Close)
	outside := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(outside, UpdatesLogsDir), defaultDirAccess))
	sentinel := filepath.Join(outside, UpdatesLogsDir, "data")
	require.NoError(t, os.WriteFile(sentinel, []byte("keep"), defaultFileAccess))
	require.NoError(t, os.Symlink(outside, filepath.Join(fs.Config.UpdatesDir(), "escape")))
	for _, update := range []string{"escape", "../../outside"} {
		_, err := fs.Updates.Logs.ReadFile(update, "data")
		require.Error(t, err)
		require.Error(t, fs.Updates.Logs.WriteFile(update, "data", "replace"))
		require.Error(t, fs.Updates.Logs.AppendFile(update, "data", "append"))
		count := 0
		for _, err := range fs.Updates.Logs.TailFileLines(update, "data", nil) {
			require.Error(t, err)
			count++
		}
		require.Equal(t, 1, count)
	}
	require.Error(t, fs.Updates.Delete("escape/logs"))
	require.Error(t, fs.Updates.Delete("../../outside"))
	require.NoError(t, fs.Updates.Logs.WriteFile("v1", "data", "safe"))
	require.NoError(t, os.Symlink(sentinel, filepath.Join(fs.Config.UpdatesDir(), "v1", UpdatesLogsDir, "link")))
	_, err = fs.Updates.Logs.ReadFile("v1", "link")
	require.Error(t, err)
	require.Error(t, fs.Updates.Logs.AppendFile("v1", "link", "append"))
	require.Error(t, fs.Updates.Logs.WriteFile("v1", "../escape", "replace"))
	require.NoError(t, os.Symlink(sentinel, filepath.Join(fs.Config.UpdatesDir(), "v1", UpdatesLogsDir, "data"+partialFileSuffix)))
	require.Error(t, fs.Updates.Logs.WriteFile("v1", "data", "replace"))
	content, err := os.ReadFile(sentinel)
	require.NoError(t, err)
	require.Equal(t, "keep", string(content))
}
