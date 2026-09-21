// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"os"
	"path/filepath"
	"testing"
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

			h := UpdatesFsHandle{
				baseFsHandle: baseFsHandle{root: tmpDir},
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

	h := UpdatesFsHandle{
		baseFsHandle: baseFsHandle{root: tmpDir},
		category:     category,
	}
	_, err := h.LatestRootMetaName(update)
	if err == nil {
		t.Fatal("expected error for empty directory, got nil")
	}
}
