// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"archive/tar"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func newTarTestHandle(t *testing.T) tarFsHandle {
	t.Helper()
	root, err := os.OpenRoot(t.TempDir())
	require.NoError(t, err)
	h := tarFsHandle{osRootHandle{root: root}}
	t.Cleanup(h.Close)
	return h
}

func tarTestFile(t *testing.T, name, content string) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	writer := tar.NewWriter(&buf)
	require.NoError(t, writer.WriteHeader(&tar.Header{Name: name, Mode: 0o640, Size: int64(len(content)), Typeflag: tar.TypeReg}))
	_, err := writer.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return &buf
}

func TestTarRootUnpack(t *testing.T) {
	for _, staged := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "staged"}[staged], func(t *testing.T) {
			h := newTarTestHandle(t)
			var opts []TarUnpackOption
			if staged {
				opts = append(opts, TarUnpackUseTmpDir("tx"), TarUnpackUseTmpFile("archive.tar"))
			}
			require.NoError(t, h.unpackTar(tarTestFile(t, "nested/file", "first"), "dest", opts...))
			content, err := h.root.ReadFile("dest/nested/file")
			require.NoError(t, err)
			require.Equal(t, "first", string(content))
			require.ErrorContains(t, h.unpackTar(tarTestFile(t, "new", "new"), "dest", opts...), "not empty")
			mergeOpts := append(append([]TarUnpackOption{}, opts...), TarUnpackMergeDest(true))
			require.NoError(t, h.unpackTar(tarTestFile(t, "new", "second"), "dest", mergeOpts...))
			content, err = h.root.ReadFile("dest/nested/file")
			require.NoError(t, err)
			require.Equal(t, "first", string(content))
			content, err = h.root.ReadFile("dest/new")
			require.NoError(t, err)
			require.Equal(t, "second", string(content))
			replaceOpts := append(append([]TarUnpackOption{}, opts...), TarUnpackReplaceDest(true))
			require.NoError(t, h.unpackTar(tarTestFile(t, "replacement", "third"), "dest", replaceOpts...))
			_, err = h.root.Stat("dest/new")
			require.ErrorIs(t, err, os.ErrNotExist)
			content, err = h.root.ReadFile("dest/replacement")
			require.NoError(t, err)
			require.Equal(t, "third", string(content))
			_, err = h.root.Stat("tx")
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

func TestTarRootRejectEscape(t *testing.T) {
	h := newTarTestHandle(t)
	outside := t.TempDir()
	sentinel := filepath.Join(outside, "sentinel")
	require.NoError(t, os.WriteFile(sentinel, []byte("keep"), 0o600))
	require.NoError(t, h.root.Mkdir("dest", 0o750))
	require.NoError(t, h.root.Symlink(outside, "dest/link"))
	require.NoError(t, h.root.Mkdir("sibling", 0o750))
	require.NoError(t, h.root.WriteFile("sibling/sentinel", []byte("sibling"), 0o600))
	require.NoError(t, h.root.Symlink("../sibling", "dest/sibling"))
	for _, name := range []string{"../outside", "nested/../../outside", sentinel, "link/sentinel", "sibling/sentinel"} {
		require.Error(t, h.unpackTar(tarTestFile(t, name, "replace"), "dest", TarUnpackMergeDest(true)), name)
	}
	require.NoError(t, h.root.Symlink(outside, "escape"))
	require.Error(t, h.unpackTar(tarTestFile(t, "file", "replace"), "escape", TarUnpackReplaceDest(true)))
	require.Error(t, h.unpackTar(tarTestFile(t, "file", "replace"), "../escape"))
	require.Error(t, h.unpackTar(tarTestFile(t, "file", "replace"), "new", TarUnpackUseTmpDir("escape")))
	require.Error(t, h.unpackTar(tarTestFile(t, "file", "replace"), "new", TarUnpackUseTmpFile("escape/sentinel")))
	// Merge copying must also reject source symlinks.
	require.Error(t, h.unpackTar(tarTestFile(t, "file", "replace"), "dest", TarUnpackMergeDest(true), TarUnpackUseTmpDir("tx")))
	data, err := os.ReadFile(sentinel)
	require.NoError(t, err)
	require.Equal(t, "keep", string(data))
	data, err = h.root.ReadFile("sibling/sentinel")
	require.NoError(t, err)
	require.Equal(t, "sibling", string(data))
}

func TestTarRootLimitsAndCleanup(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts []TarUnpackOption
		want error
	}{
		{"file", []TarUnpackOption{TarUnpackMaxItemFileSize(1)}, ErrTarUnpackExceedsLimitFileSize},
		{"disk", []TarUnpackOption{TarUnpackMaxTotalDiskSize(1)}, ErrTarUnpackExceedsLimitDiskSize},
		{"raw", []TarUnpackOption{TarUnpackMaxTotalDiskSize(1), TarUnpackUseTmpFile("archive.tar")}, ErrTarUnpackExceedsLimitRawSize},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newTarTestHandle(t)
			opts := append(tc.opts, TarUnpackUseTmpDir("tx"))
			require.ErrorIs(t, h.unpackTar(tarTestFile(t, "file", "content"), "dest", opts...), tc.want)
			_, err := h.root.Stat("tx")
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
	h := newTarTestHandle(t)
	rejected := errors.New("reject archive")
	require.ErrorIs(t, h.unpackTar(tarTestFile(t, "file", "content"), "dest",
		TarUnpackUseTmpDir("tx"), TarUnpackOnEvents(tarUnpackEvents{
			onUnpackComplete: func() error { return rejected },
		})), rejected)
	_, err := h.root.Stat("tx")
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestTarRootPreservesBackupOnCommitFailure(t *testing.T) {
	h := newTarTestHandle(t)
	require.NoError(t, h.root.Mkdir("dest", 0o750))
	require.NoError(t, h.root.WriteFile("dest/old", []byte("keep"), 0o640))
	commitFailed := errors.New("commit failed")
	var callbackCalled bool
	err := h.unpackTar(tarTestFile(t, "new", "new"), "dest",
		TarUnpackMergeDest(true), TarUnpackUseTmpDir("tx"),
		TarUnpackOnEvents(tarUnpackEvents{
			onUnpackComplete: func() error {
				return h.root.Rename("tx/unpacked", "tx/saved")
			},
			onTmpRenameError: func(err error) (bool, error) {
				require.ErrorIs(t, err, os.ErrNotExist)
				callbackCalled = true
				return true, commitFailed
			},
		}))
	require.ErrorIs(t, err, commitFailed)
	require.True(t, callbackCalled)
	content, err := h.root.ReadFile("tx/backup/old")
	require.NoError(t, err)
	require.Equal(t, "keep", string(content))
}
