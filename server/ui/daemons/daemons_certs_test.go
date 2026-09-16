// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package daemons

import (
	"bytes"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	projectContext "github.com/foundriesio/update-server/context"
	"github.com/foundriesio/update-server/storage"
	apiStorage "github.com/foundriesio/update-server/storage/api"
	"github.com/stretchr/testify/require"
)

func TestCertGcDaemon(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := storage.NewDb(filepath.Join(tmpDir, storage.DbFile))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	fs, err := storage.NewFs(tmpDir)
	require.NoError(t, err)
	api, err := apiStorage.NewStorage(db, fs)
	require.NoError(t, err)

	insert, err := db.Prepare("TestInsertOldCert", `
		INSERT INTO old_certs(expires, sha1) VALUES (?, ?)`)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, insert.Close())
	})
	count, err := db.Prepare("TestCountOldCerts", "SELECT COUNT(*) FROM old_certs")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, count.Close())
	})

	now := time.Now().Unix()
	_, err = insert.Exec(now-1, bytes.Repeat([]byte{1}, 20))
	require.NoError(t, err)
	_, err = insert.Exec(now+3600, bytes.Repeat([]byte{2}, 20))
	require.NoError(t, err)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := daemons{
		context: projectContext.CtxWithLog(projectContext.Background(), log),
		storage: api,
	}
	WithCertGcInterval(20 * time.Millisecond)(&d)
	stop := make(chan bool)
	done := make(chan struct{})
	go func() {
		d.certGcDaemon()(stop)
		close(done)
	}()
	t.Cleanup(func() {
		close(stop)
		<-done
	})

	oldCertCount := func() int {
		var n int
		require.NoError(t, count.QueryRow().Scan(&n))
		return n
	}
	require.Eventually(t, func() bool {
		return oldCertCount() == 1
	}, time.Second, 10*time.Millisecond)

	_, err = insert.Exec(now-1, bytes.Repeat([]byte{3}, 20))
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return oldCertCount() == 1
	}, time.Second, 10*time.Millisecond)
}
