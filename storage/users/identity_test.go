// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package users

import (
	"path/filepath"
	"testing"

	"github.com/foundriesio/update-server/storage"
	"github.com/stretchr/testify/require"
)

func identityStorage(t *testing.T) *Storage {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.NewDb(filepath.Join(dir, "sql.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	fs, err := storage.NewFs(dir)
	require.NoError(t, err)
	require.NoError(t, fs.Auth.InitHmacSecret())
	s, err := NewStorage(db, fs, &storage.AuthConfig{})
	require.NoError(t, err)
	return s
}

func TestIdentityIsolation(t *testing.T) {
	s := identityStorage(t)
	local := &User{Username: "alice", AllowedScopes: ScopeUsersRU}
	require.NoError(t, s.Create(local))
	a, err := s.UpsertIdentity("google", "123", "alice", "alice@a.com", ScopeDevicesR)
	require.NoError(t, err)
	require.NotEqual(t, local.id, a.id)
	require.Equal(t, "alice-2", a.Username)
	require.Equal(t, ScopeDevicesR, a.AllowedScopes)

	b, err := s.UpsertIdentity("google", "456", "alice", "alice@b.com", ScopeDevicesD)
	require.NoError(t, err)
	require.NotEqual(t, a.id, b.id)
	require.Equal(t, "alice-3", b.Username)

	c, err := s.UpsertIdentity("github", "123", "alice", "alice@a.com", ScopeUsersR)
	require.NoError(t, err)
	require.NotEqual(t, a.id, c.id)
	require.Equal(t, ScopeUsersR, c.AllowedScopes)

	// A rename to another account's username must still resolve by subject.
	a.AllowedScopes = ScopeUpdatesRU
	require.NoError(t, a.Update("Change permissions"))
	renamed, err := s.UpsertIdentity("google", "123", b.Username, "renamed@a.com", ScopeUsersRU)
	require.NoError(t, err)
	require.Equal(t, a.id, renamed.id)
	require.Equal(t, a.Username, renamed.Username)
	require.Equal(t, "renamed@a.com", renamed.Email)
	require.Equal(t, ScopeUpdatesRU, renamed.AllowedScopes)

	reused, err := s.UpsertIdentity("google", "789", "alice", "alice@a.com", ScopeDevicesR)
	require.NoError(t, err)
	require.NotEqual(t, a.id, reused.id)
	require.Equal(t, ScopeDevicesR, reused.AllowedScopes)

	require.NoError(t, a.Delete())
	deleted, err := s.UpsertIdentity("google", "123", "other", "other@a.com", ScopeUsersRU)
	require.ErrorIs(t, err, ErrIdentityDeleted)
	require.Nil(t, deleted)

	for _, pair := range [][2]string{{"", "123"}, {"google", ""}, {"google", " "}} {
		_, err := s.UpsertIdentity(pair[0], pair[1], "bad", "", 0)
		require.Error(t, err)
	}
	all, err := s.List()
	require.NoError(t, err)
	require.Len(t, all, 4)
}

func TestConcurrentIdentityCreation(t *testing.T) {
	s := identityStorage(t)
	type result struct {
		user *User
		err  error
	}
	results := make(chan result, 12)
	for range 12 {
		go func() {
			u, err := s.UpsertIdentity("github", "123", "alice", "a@example.com", ScopeDevicesR)
			results <- result{u, err}
		}()
	}
	var id int64
	for range 12 {
		r := <-results
		require.NoError(t, r.err)
		if id == 0 {
			id = r.user.id
		}
		require.Equal(t, id, r.user.id)
	}
	all, err := s.List()
	require.NoError(t, err)
	require.Len(t, all, 1)
}
