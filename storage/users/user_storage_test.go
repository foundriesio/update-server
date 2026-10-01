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
	require.NoError(t, err)
	fs, err := storage.NewFs(tmpdir)
	require.NoError(t, err)

	require.NoError(t, fs.Auth.InitHmacSecret())

	users, err := NewStorage(db, fs, nil)
	require.NoError(t, err)
	require.NotNil(t, users)

	u := User{
		Username:      "testuser",
		Password:      "passwordhash",
		Email:         "testuser@example.com",
		AllowedScopes: ScopeDevicesR | ScopeUsersRU,
	}
	now := time.Now().Unix()
	err = users.Create(&u)
	require.NoError(t, err)
	require.NotZero(t, u.id)
	require.InDelta(t, now, u.CreatedAt, 5)

	u2, err := users.Get("testuser")
	require.NoError(t, err)
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
	require.NoError(t, err)
	require.Nil(t, u3)

	ul, err := users.List()
	require.NoError(t, err)
	require.Len(t, ul, 1)
	require.Equal(t, u.Username, ul[0].Username)

	type authData struct {
		ID string
	}
	u.Username = "seconduser"
	u.AuthProviderData, err = json.Marshal(authData{ID: "auth-123"})
	require.NoError(t, err)
	err = users.Create(&u)
	require.NoError(t, err)

	u2, err = users.Get("seconduser")
	require.NoError(t, err)
	require.NotNil(t, u2)
	data := authData{}
	require.NoError(t, json.Unmarshal(u2.AuthProviderData, &data))
	require.Equal(t, "auth-123", data.ID)

	ul, err = users.List()
	require.NoError(t, err)
	require.Len(t, ul, 2)

	require.NoError(t, u.Delete())
	ul, err = users.List()
	require.NoError(t, err)
	require.Len(t, ul, 1)
	require.Equal(t, "testuser", ul[0].Username)

	ul[0].AllowedScopes = ScopeDevicesD
	require.NoError(t, ul[0].Update("changed scopes"))

	u4, err := users.Get("testuser")
	require.NoError(t, err)
	require.NotNil(t, u4)
	require.Equal(t, "devices:delete", u4.AllowedScopes.String())
}

func TestTokens(t *testing.T) {
	tmpdir := t.TempDir()
	dbFile := filepath.Join(tmpdir, "sql.db")
	db, err := storage.NewDb(dbFile)
	require.NoError(t, err)
	fs, err := storage.NewFs(tmpdir)
	require.NoError(t, err)
	require.NoError(t, fs.Auth.InitHmacSecret())

	cfg := storage.AuthConfig{}
	users, err := NewStorage(db, fs, &cfg)
	require.NoError(t, err)
	require.NotNil(t, users)

	u := User{
		Username:      "testuser",
		Password:      "passwordhash",
		Email:         "testuser@example.com",
		AllowedScopes: ScopeDevicesRU,
	}
	err = users.Create(&u)
	require.NoError(t, err)

	expires := time.Now().Add(1 * time.Hour).Unix()
	t1, err := u.GenerateToken("desc", expires, ScopeDevicesR)
	require.NoError(t, err)

	cfg.MaxTokenLifetimeDays = 1
	expires = time.Now().Add(36 * time.Hour).Unix()
	_, err = u.GenerateToken("desc", expires, ScopeDevicesR)
	require.NotNil(t, err)
	require.Contains(t, err.Error(), "requested expiration exceeds maximum allowed expiration 1 days")

	expires = time.Now().Add(1 * time.Hour).Unix()

	time.Sleep(time.Second)
	expired := time.Now().Add(-1 * time.Hour).Unix()
	t2, err := u.GenerateToken("desc2", expired, ScopeDevicesR)
	require.NoError(t, err)
	require.NotEqual(t, t1.Value, t2.Value)

	u2, err := users.GetByToken(t1.Value)
	require.NoError(t, err)
	require.NotNil(t, u2)
	require.Equal(t, u.id, u2.id)
	require.True(t, u2.AllowedScopes.Has(ScopeDevicesR))
	require.False(t, u2.AllowedScopes.Has(ScopeDevicesRU))

	u2, err = users.GetByToken(t2.Value)
	require.NoError(t, err)
	require.Nil(t, u2)

	tokens, err := u.ListTokens()
	require.NoError(t, err)
	require.Len(t, tokens, 2)

	require.Equal(t, t1.PublicID, tokens[0].PublicID)
	require.Equal(t, t2.PublicID, tokens[1].PublicID)
	require.NoError(t, u.DeleteToken(tokens[1].PublicID))

	tokens, err = u.ListTokens()
	require.NoError(t, err)
	require.Len(t, tokens, 1)

	require.NoError(t, u.Delete())
	tokens, err = u.ListTokens()
	require.NoError(t, err)
	require.Len(t, tokens, 0)

	_, err = u.GenerateToken("invalid scope", expires, ScopeUsersC)
	require.NotNil(t, err)

	// Generate token with read-update
	t1, err = u.GenerateToken("desc", expires, ScopeDevicesRU)
	require.NoError(t, err)
	// Downgrade user to devices:read
	u.AllowedScopes = ScopeDevicesR
	require.NoError(t, u.Update("test"))
	u2, err = users.GetByToken(t1.Value)
	require.NoError(t, err)
	require.True(t, u2.AllowedScopes.Has(ScopeDevicesR))
	require.False(t, u2.AllowedScopes.Has(ScopeDevicesRU))

	events, err := fs.Audit.ReadEvents(u.id)
	require.NoError(t, err)
	require.Contains(t, events, "User created")
	require.Contains(t, events, "Token created")
	require.Contains(t, events, "Token deleted id=")
	require.Contains(t, events, "User deleted")
}

func TestGc(t *testing.T) {
	tmpdir := t.TempDir()
	dbFile := filepath.Join(tmpdir, "sql.db")
	db, err := storage.NewDb(dbFile)
	require.NoError(t, err)
	fs, err := storage.NewFs(tmpdir)
	require.NoError(t, err)
	require.NoError(t, fs.Auth.InitHmacSecret())

	users, err := NewStorage(db, fs, &storage.AuthConfig{})
	require.NoError(t, err)
	require.NotNil(t, users)

	u := User{
		Username:      "testuser",
		Password:      "passwordhash",
		Email:         "testuser@example.com",
		AllowedScopes: ScopeDevicesRU,
	}
	err = users.Create(&u)
	require.NoError(t, err)

	expires := time.Now().Add(-time.Hour).Unix()
	_, err = u.GenerateToken("desc", expires, ScopeDevicesR)
	require.NoError(t, err)

	session, err := u.CreateSession("127.0.0.1", expires, ScopeDevicesR)
	require.NoError(t, err)
	require.NotEmpty(t, session)

	users.RunGc()

	tokens, err := u.ListTokens()
	require.NoError(t, err)
	require.Len(t, tokens, 0)

	u2, err := users.GetBySession(session)
	require.NoError(t, err)
	require.Nil(t, u2)
}

func TestOAuth2DeviceFlow(t *testing.T) {
	tmpdir := t.TempDir()
	dbFile := filepath.Join(tmpdir, "sql.db")
	db, err := storage.NewDb(dbFile)
	require.NoError(t, err)
	fs, err := storage.NewFs(tmpdir)
	require.NoError(t, err)
	require.NoError(t, fs.Auth.InitHmacSecret())

	users, err := NewStorage(db, fs, &storage.AuthConfig{})
	require.NoError(t, err)
	require.NotNil(t, users)

	u := User{
		Username:      "testuser",
		Password:      "passwordhash",
		Email:         "testuser@example.com",
		AllowedScopes: ScopeDevicesRU,
	}
	err = users.Create(&u)
	require.NoError(t, err)

	// Test creating device authorization
	now := time.Now().Unix()
	expiresAt := now + 600     // 10 minutes
	tokenExpires := now + 3600 // 1 hour
	scopes := "devices:read"

	deviceCode, userCode, err := users.CreateDeviceAuth(expiresAt, tokenExpires, scopes)
	require.NoError(t, err)

	// Test getting device auth by device code
	auth, err := users.GetDeviceAuthByDeviceCode(deviceCode)
	require.NoError(t, err)
	require.NotNil(t, auth)
	require.Equal(t, deviceCode, auth.DeviceCode)
	require.Equal(t, userCode, auth.UserCode)
	require.Equal(t, expiresAt, auth.ExpiresAt)
	require.Equal(t, tokenExpires, auth.TokenExpires)
	require.Equal(t, scopes, auth.Scopes)
	require.Nil(t, auth.UserID)
	require.False(t, auth.Authorized)
	require.False(t, auth.Denied)

	// Test getting device auth by user code
	auth2, err := users.GetDeviceAuthByUserCode(userCode)
	require.NoError(t, err)
	require.NotNil(t, auth2)
	require.Equal(t, deviceCode, auth2.DeviceCode)
	require.Equal(t, userCode, auth2.UserCode)

	// Test getting non-existent device auth
	auth3, err := users.GetDeviceAuthByDeviceCode("nonexistent")
	require.NoError(t, err)
	require.Nil(t, auth3)

	auth4, err := users.GetDeviceAuthByUserCode("ZZZZ-ZZZZ")
	require.NoError(t, err)
	require.Nil(t, auth4)

	// Test approving authorization
	err = u.ApproveAuthorization(deviceCode, "test token description", ScopeDevicesR)
	require.NoError(t, err)

	auth5, err := users.GetDeviceAuthByDeviceCode(deviceCode)
	require.NoError(t, err)
	require.NotNil(t, auth5)
	require.True(t, auth5.Authorized)
	require.False(t, auth5.Denied)
	require.NotNil(t, auth5.UserID)
	require.Equal(t, u.id, *auth5.UserID)
	require.Equal(t, "test token description", auth5.TokenDescription)

	// Create another device auth for deny test
	deviceCode2, _, err := users.CreateDeviceAuth(expiresAt, tokenExpires, scopes)
	require.NoError(t, err)

	// Test denying authorization
	err = u.DenyDeviceAuth(deviceCode2)
	require.NoError(t, err)

	auth6, err := users.GetDeviceAuthByDeviceCode(deviceCode2)
	require.NoError(t, err)
	require.NotNil(t, auth6)
	require.False(t, auth6.Authorized)
	require.True(t, auth6.Denied)
	require.Nil(t, auth6.UserID)

	// Test deleting expired device auth entries
	expiredExpiresAt := now - 600
	deviceCode3, _, err := users.CreateDeviceAuth(expiredExpiresAt, tokenExpires, scopes)
	require.NoError(t, err)

	// Verify it exists
	auth7, err := users.GetDeviceAuthByDeviceCode(deviceCode3)
	require.NoError(t, err)
	require.NotNil(t, auth7)

	// Delete expired entries
	err = users.DeleteExpiredDeviceAuth(now)
	require.NoError(t, err)

	// Verify expired entry is gone
	auth8, err := users.GetDeviceAuthByDeviceCode(deviceCode3)
	require.NoError(t, err)
	require.Nil(t, auth8)

	// Verify non-expired entries still exist
	auth9, err := users.GetDeviceAuthByDeviceCode(deviceCode)
	require.NoError(t, err)
	require.NotNil(t, auth9)

	auth10, err := users.GetDeviceAuthByDeviceCode(deviceCode2)
	require.NoError(t, err)
	require.NotNil(t, auth10)
}
