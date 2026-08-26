// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	storageTesting "github.com/foundriesio/update-server/storage/testing"
)

func TestUnpackTarMaxSize(t *testing.T) {
	t.Run("under limit succeeds and returns total bytes", func(t *testing.T) {
		tmpDir := t.TempDir()
		h := tarFsHandle{root: tmpDir}
		buf := storageTesting.CreateTarBuffer(t, map[string]string{
			"a.txt": "hello",
			"b.txt": "world!",
		})

		size, err := h.unpackTar(buf, "dest", TarUnpackMaxSize(100))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := int64(len("hello") + len("world!")); size != want {
			t.Errorf("got size %d, want %d", size, want)
		}
	})

	t.Run("single file exceeding limit is rejected", func(t *testing.T) {
		tmpDir := t.TempDir()
		h := tarFsHandle{root: tmpDir}
		buf := storageTesting.CreateTarBuffer(t, map[string]string{
			"big.txt": strings.Repeat("x", 1000),
		})

		_, err := h.unpackTar(buf, "dest", TarUnpackMaxSize(10))
		if !errors.Is(err, ErrUnpackTooLarge) {
			t.Fatalf("expected ErrUnpackTooLarge, got: %v", err)
		}
	})

	t.Run("many small files summing over limit is rejected", func(t *testing.T) {
		tmpDir := t.TempDir()
		h := tarFsHandle{root: tmpDir}
		files := map[string]string{}
		for i := range 20 {
			files[strings.Repeat("f", 1)+string(rune('a'+i))+".txt"] = "12345"
		}
		buf := storageTesting.CreateTarBuffer(t, files)

		// 20 files * 5 bytes = 100 bytes total, well over a 10 byte budget.
		_, err := h.unpackTar(buf, "dest", TarUnpackMaxSize(10))
		if !errors.Is(err, ErrUnpackTooLarge) {
			t.Fatalf("expected ErrUnpackTooLarge, got: %v", err)
		}
	})

	t.Run("tmp dir is cleaned up after rejection", func(t *testing.T) {
		tmpDir := t.TempDir()
		h := tarFsHandle{root: tmpDir}
		buf := storageTesting.CreateTarBuffer(t, map[string]string{
			"big.txt": strings.Repeat("x", 1000),
		})

		const txDir = ".test-upload-tx"
		_, err := h.unpackTar(buf, "dest", TarUnpackMaxSize(10), TarUnpackUseTmpDir(txDir))
		if !errors.Is(err, ErrUnpackTooLarge) {
			t.Fatalf("expected ErrUnpackTooLarge, got: %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(tmpDir, txDir)); !os.IsNotExist(statErr) {
			t.Errorf("expected tmp dir %q to be cleaned up, stat err: %v", txDir, statErr)
		}
	})

	t.Run("zero max size means unlimited", func(t *testing.T) {
		tmpDir := t.TempDir()
		h := tarFsHandle{root: tmpDir}
		content := strings.Repeat("x", 1000)
		buf := storageTesting.CreateTarBuffer(t, map[string]string{
			"big.txt": content,
		})

		size, err := h.unpackTar(buf, "dest")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if size != int64(len(content)) {
			t.Errorf("got size %d, want %d", size, len(content))
		}
	})
}
