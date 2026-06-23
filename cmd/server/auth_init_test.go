// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/foundriesio/update-server/storage"
)

func TestAuthInitLocal(t *testing.T) {
	tmpDir := t.TempDir()

	cmd := AuthInitCmd{Local: true}
	common := CommonArgs{DataDir: tmpDir}
	require.Nil(t, cmd.Run(common))

	fs, err := storage.NewFs(tmpDir)
	require.Nil(t, err)

	cfg, err := fs.Auth.GetAuthConfig()
	require.Nil(t, err)
	require.Equal(t, "local", cfg.Type)
	require.Greater(t, len(cfg.NewUserDefaultScopes), 0)
	require.Greater(t, len(cfg.Config), 0)
}
