// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"crypto/x509"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/users"
)

func TestDevServerInit(t *testing.T) {
	tmpDir := t.TempDir()

	cmd := DevServerInitCmd{
		DnsName:       "example.com",
		Factory:       "example",
		AdminPassword: "supersecret",
	}
	common := CommonArgs{DataDir: tmpDir}
	require.Nil(t, cmd.Run(common))

	fs, err := storage.NewFs(common.DataDir)
	require.Nil(t, err)

	// PKI was created.
	rootCrt, err := storage.LoadPemFile(fs.Certs.FilePath(storage.CertsRootPemFile), x509.ParseCertificate)
	require.Nil(t, err)
	require.Equal(t, []string{"example"}, rootCrt.Subject.OrganizationalUnit)

	// Local auth was configured.
	cfg, err := fs.Auth.GetAuthConfig()
	require.Nil(t, err)
	require.Equal(t, "local", cfg.Type)

	// TUF was initialized (re-initializing fails).
	require.ErrorIs(t, fs.Tuf.InitTuf(), storage.ErrTufAlreadyInitialized)

	// The admin user exists.
	db, err := storage.NewDb(fs.Config.DbFile())
	require.Nil(t, err)
	userStorage, err := users.NewStorage(db, fs, cfg)
	require.Nil(t, err)
	u, err := userStorage.Get("admin")
	require.Nil(t, err)
	require.NotNil(t, u)

	// Re-running against an already-initialized data dir fails.
	require.Error(t, cmd.Run(common))
}
