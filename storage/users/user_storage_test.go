// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package users

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/foundriesio/update-server/storage"
	"github.com/stretchr/testify/require"
)

func TestNewStorage(t *testing.T) {
	tmpdir := t.TempDir()
	dbFile := filepath.Join(tmpdir, "sql.db")
	db, err := storage.NewDb(dbFile)
	require.Nil(t, err)
	fs, err := storage.NewFs(tmpdir)
	require.Nil(t, err)

	require.Nil(t, fs.Auth.InitHmacSecret())

	users, err := NewStorage(db, fs)
	require.Nil(t, err)
	require.NotNil(t, users)

	u := User{
		Username:      "testuser",
		Password:      "passwordhash",
		Email:         "testuser@example.com",
		AllowedScopes: ScopeDevicesR | ScopeUsersRU,
	}
	now := time.Now().Unix()
	err = users.Create(&u)
	require.Nil(t, err)
	require.NotZero(t, u.id)
	require.InDelta(t, now, u.CreatedAt, 5)

	u2, err := users.Get("testuser")
	require.Nil(t, err)
	require.NotNil(t, u2)
	require.Equal(t, u.id, u2.id)
	require.Equal(t, u.Username, u2.Username)
	require.Equal(t, u.Password, u2.Password)
	require.Equal(t, u.Email, u2.Email)
	require.Equal(t, u.AllowedScopes, u2.AllowedScopes)

	require.True(t, u2.AllowedScopes.Has(ScopeDevicesR))
	require.False(t, u2.AllowedScopes.Has(ScopeDevicesD))
	require.Equal(t, []string{"devices:read", "users:read-update"}, u2.AllowedScopes.ToSlice())

	require.NotNil(t, users.Create(u2), "duplicate username should fail")

	u3, err := users.Get("nonexistent")
	require.Nil(t, err)
	require.Nil(t, u3)

	ul, err := users.List()
	require.Nil(t, err)
	require.Len(t, ul, 1)
	require.Equal(t, u.Username, ul[0].Username)

	type authData struct {
		ID string
	}
	u.Username = "seconduser"
	u.AuthProviderData, err = json.Marshal(authData{ID: "auth-123"})
	require.Nil(t, err)
	err = users.Create(&u)
	require.Nil(t, err)

	u2, err = users.Get("seconduser")
	require.Nil(t, err)
	require.NotNil(t, u2)
	data := authData{}
	require.Nil(t, json.Unmarshal(u2.AuthProviderData, &data))
	require.Equal(t, "auth-123", data.ID)

	ul, err = users.List()
	require.Nil(t, err)
	require.Len(t, ul, 2)

	require.Nil(t, u.Delete())
	ul, err = users.List()
	require.Nil(t, err)
	require.Len(t, ul, 1)
	require.Equal(t, "testuser", ul[0].Username)

	ul[0].AllowedScopes = ScopeDevicesD
	require.Nil(t, ul[0].Update("changed scopes"))

	u4, err := users.Get("testuser")
	require.Nil(t, err)
	require.NotNil(t, u4)
	require.Equal(t, "devices:delete", u4.AllowedScopes.String())
}

func TestTokens(t *testing.T) {
	tmpdir := t.TempDir()
	dbFile := filepath.Join(tmpdir, "sql.db")
	db, err := storage.NewDb(dbFile)
	require.Nil(t, err)
	fs, err := storage.NewFs(tmpdir)
	require.Nil(t, err)
	require.Nil(t, fs.Auth.InitHmacSecret())

	users, err := NewStorage(db, fs)
	require.Nil(t, err)
	require.NotNil(t, users)

	u := User{
		Username:      "testuser",
		Password:      "passwordhash",
		Email:         "testuser@example.com",
		AllowedScopes: ScopeDevicesRU,
	}
	err = users.Create(&u)
	require.Nil(t, err)

	expires := time.Now().Add(1 * time.Hour).Unix()
	t1, err := u.GenerateToken("desc", expires, ScopeDevicesR)
	require.Nil(t, err)

	time.Sleep(time.Second)
	expired := time.Now().Add(-1 * time.Hour).Unix()
	t2, err := u.GenerateToken("desc2", expired, ScopeDevicesR)
	require.Nil(t, err)
	require.NotEqual(t, t1.Value, t2.Value)

	u2, err := users.GetByToken(t1.Value)
	require.Nil(t, err)
	require.NotNil(t, u2)
	require.Equal(t, u.id, u2.id)
	require.True(t, u2.AllowedScopes.Has(ScopeDevicesR))
	require.False(t, u2.AllowedScopes.Has(ScopeDevicesRU))

	u2, err = users.GetByToken(t2.Value)
	require.Nil(t, err)
	require.Nil(t, u2)

	tokens, err := u.ListTokens()
	require.Nil(t, err)
	require.Len(t, tokens, 2)

	require.Equal(t, t1.PublicID, tokens[0].PublicID)
	require.Equal(t, t2.PublicID, tokens[1].PublicID)
	require.Nil(t, u.DeleteToken(tokens[1].PublicID))

	tokens, err = u.ListTokens()
	require.Nil(t, err)
	require.Len(t, tokens, 1)

	require.Nil(t, u.Delete())
	tokens, err = u.ListTokens()
	require.Nil(t, err)
	require.Len(t, tokens, 0)

	_, err = u.GenerateToken("invalid scope", expires, ScopeUsersC)
	require.NotNil(t, err)

	// Generate token with read-update
	t1, err = u.GenerateToken("desc", expires, ScopeDevicesRU)
	require.Nil(t, err)
	// Downgrade user to devices:read
	u.AllowedScopes = ScopeDevicesR
	require.Nil(t, u.Update("test"))
	u2, err = users.GetByToken(t1.Value)
	require.Nil(t, err)
	require.True(t, u2.AllowedScopes.Has(ScopeDevicesR))
	require.False(t, u2.AllowedScopes.Has(ScopeDevicesRU))

	events, err := fs.Audit.ReadEvents(u.id)
	require.Nil(t, err)
	require.Contains(t, events, "User created")
	require.Contains(t, events, "Token created")
	require.Contains(t, events, "Token deleted id=")
	require.Contains(t, events, "User deleted")
}

func TestGc(t *testing.T) {
	tmpdir := t.TempDir()
	dbFile := filepath.Join(tmpdir, "sql.db")
	db, err := storage.NewDb(dbFile)
	require.Nil(t, err)
	fs, err := storage.NewFs(tmpdir)
	require.Nil(t, err)
	require.Nil(t, fs.Auth.InitHmacSecret())

	users, err := NewStorage(db, fs)
	require.Nil(t, err)
	require.NotNil(t, users)

	u := User{
		Username:      "testuser",
		Password:      "passwordhash",
		Email:         "testuser@example.com",
		AllowedScopes: ScopeDevicesRU,
	}
	err = users.Create(&u)
	require.Nil(t, err)

	expires := time.Now().Add(-time.Hour).Unix()
	_, err = u.GenerateToken("desc", expires, ScopeDevicesR)
	require.Nil(t, err)

	session, err := u.CreateSession("127.0.0.1", expires, ScopeDevicesR)
	require.Nil(t, err)
	require.NotEmpty(t, session)

	users.RunGc()

	tokens, err := u.ListTokens()
	require.Nil(t, err)
	require.Len(t, tokens, 0)

	u2, err := users.GetBySession(session)
	require.Nil(t, err)
	require.Nil(t, u2)
}
