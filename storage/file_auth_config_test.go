// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthHmacSecret(t *testing.T) {
	fs, err := NewFs(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(fs.Close)

	_, err = fs.Auth.GetHmacSecret()
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, fs.Auth.InitHmacSecret())
	secret, err := fs.Auth.GetHmacSecret()
	require.NoError(t, err)
	require.Len(t, secret, 64)
	info, err := os.Stat(filepath.Join(fs.Config.AuthDir(), HmacFile))
	require.NoError(t, err)
	require.Equal(t, secureFileAccess, info.Mode().Perm())

	require.ErrorContains(t, fs.Auth.InitHmacSecret(), "hmac secret exists")
	again, err := fs.Auth.GetHmacSecret()
	require.NoError(t, err)
	require.Equal(t, secret, again)
}

func TestAuthConfigRoundTrip(t *testing.T) {
	fs, err := NewFs(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(fs.Close)

	cfg := AuthConfig{
		Type:                 "local",
		MaxTokenLifetimeDays: 30,
		NewUserDefaultScopes: []string{"devices:read"},
		RateLimits:           RateLimitConfig{BadAuthLimit: 5},
		Config:               json.RawMessage(`{"users": []}`),
	}
	for _, timeout := range []int{0, 12} {
		cfg.SessionTimeoutHours = timeout
		require.NoError(t, fs.Auth.SaveAuthConfig(cfg))
		got, err := fs.Auth.GetAuthConfig()
		require.NoError(t, err)
		require.JSONEq(t, string(cfg.Config), string(got.Config))
		want := cfg
		want.Config = got.Config
		if timeout == 0 {
			want.SessionTimeoutHours = 48
		}
		require.Equal(t, want, *got)
	}
}

func TestAuthConfigReadErrors(t *testing.T) {
	fs, err := NewFs(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(fs.Close)

	_, err = fs.Auth.GetAuthConfig()
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, os.WriteFile(filepath.Join(fs.Config.AuthDir(), AuthConfigFile), []byte("{"), defaultFileAccess))
	_, err = fs.Auth.GetAuthConfig()
	require.ErrorContains(t, err, "unable to unmarshall auth config")
}
