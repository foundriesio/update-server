// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

//go:build !nodb

package storage

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Users table as it was before the locale column was added.
const usersTableBeforeLocale = `
	CREATE TABLE users (
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		username       TEXT NOT NULL UNIQUE,
		password       VARCHAR(128),
		email          TEXT,
		created_at     INT DEFAULT 0,
		deleted        BOOL DEFAULT 0,
		allowed_scopes TEXT DEFAULT '',

		auth_provider_data JSONB NOT NULL DEFAULT '{}'
	);
	INSERT INTO users (username, password, email, created_at, allowed_scopes)
	VALUES ('olduser', 'pw', 'old@example.com', 1, 'devices:read');`

func createUsersDbBeforeLocale(t *testing.T) string {
	t.Helper()
	dbFile := filepath.Join(t.TempDir(), "sql.db")
	old, err := sql.Open("sqlite3", dbFile)
	require.NoError(t, err)
	_, err = old.Exec(usersTableBeforeLocale)
	require.NoError(t, err)
	require.NoError(t, old.Close())
	return dbFile
}

func openDb(t *testing.T, dbFile string) *DbHandle {
	t.Helper()
	db, err := NewDb(dbFile)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

func requireLocaleColumn(t *testing.T, db *DbHandle) {
	t.Helper()
	var notNull int
	var dflt sql.NullString
	err := db.db.QueryRow(
		`SELECT "notnull", dflt_value FROM pragma_table_info('users') WHERE name = 'locale'`,
	).Scan(&notNull, &dflt)
	require.NoError(t, err)
	require.Equal(t, 1, notNull)
	require.Equal(t, "''", dflt.String)
}

func TestNewDbFreshHasLocaleColumn(t *testing.T) {
	db := openDb(t, filepath.Join(t.TempDir(), "sql.db"))
	requireLocaleColumn(t, db)
}

func TestNewDbUpgradesExistingUsersTable(t *testing.T) {
	dbFile := createUsersDbBeforeLocale(t)
	db := openDb(t, dbFile)
	requireLocaleColumn(t, db)

	var username, locale string
	require.NoError(t, db.db.QueryRow(
		`SELECT username, locale FROM users WHERE username = 'olduser'`,
	).Scan(&username, &locale))
	require.Equal(t, "olduser", username)
	require.Empty(t, locale)

	// Reopening must skip the ALTER; a second one fails with duplicate column.
	openDb(t, dbFile)
}

func TestOldUserStatementsRunAfterUpgrade(t *testing.T) {
	db := openDb(t, createUsersDbBeforeLocale(t))

	// Statement text as it was before the locale column existed.
	_, err := db.db.Exec(
		`UPDATE users SET username = ?, password = ?, email = ?, allowed_scopes = ?, deleted = ?, auth_provider_data = jsonb(?) WHERE id = ?`,
		"olduser", "pw2", "old@example.com", "devices:read", false, "{}", 1,
	)
	require.NoError(t, err)

	var username, allowedScopes, authData string
	require.NoError(t, db.db.QueryRow(
		`SELECT id, username, password, email, created_at, allowed_scopes, json_extract(auth_provider_data, '$') FROM users WHERE id = ? and deleted = false`,
		1,
	).Scan(new(int64), &username, new(string), new(string), new(int64), &allowedScopes, &authData))
	require.Equal(t, "olduser", username)
	require.Equal(t, "devices:read", allowedScopes)
}
