// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

//go:build !nodb

package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"os"

	sqllite "github.com/mattn/go-sqlite3"
)

type DbHandle struct {
	db *sql.DB
}

var (
	ErrDbConstraintUnique = sqllite.ErrConstraintUnique
)

func IsDbError(err error, code sqllite.ErrNoExtended) bool {
	// errors.Is does not help with sqllite error codes, as they are numbers, not errors
	var e sqllite.Error
	if errors.As(err, &e) {
		return e.ExtendedCode == code
	}
	return false
}

func NewDb(dbfile string) (*DbHandle, error) {
	var newDb bool
	if _, err := os.Stat(dbfile); err != nil {
		newDb = errors.Is(err, os.ErrNotExist)
	}
	db, err := sql.Open("sqlite3", dbfile)
	if err != nil {
		return nil, err
	}
	if newDb {
		if err := createTables(db); err != nil {
			return nil, err
		}
	}
	return &DbHandle{db: db}, nil
}

func (d DbHandle) Close() error {
	return d.db.Close()
}

func (d DbHandle) Query(query string, args ...any) (*sql.Rows, error) {
	return d.db.Query(query, args...)
}

func (d DbHandle) QueryRow(query string, args ...any) *sql.Row {
	return d.db.QueryRow(query, args...)
}

func (d DbHandle) Prepare(name, query string) (stmt *sql.Stmt, err error) {
	if stmt, err = d.db.Prepare(query); err != nil {
		err = fmt.Errorf("unable to prepare '%s' statement: %w", name, err)
	}
	return
}

func (d DbHandle) InitStmt(stmt ...DbStmtInit) (err error) {
	for _, s := range stmt {
		if err = s.Init(d); err != nil {
			break
		}
	}
	return
}

func createTables(db *sql.DB) error {
	sqlStmt := `
		CREATE TABLE devices (
			uuid VARCHAR(48) NOT NULL PRIMARY KEY,
			pubkey TEXT,
			deleted BOOL,
			is_prod BOOL,
			created_at INT DEFAULT 0,
			last_seen INT DEFAULT 0,
			tag VARCHAR(80) DEFAULT "",
			labels JSONB(2048) DEFAULT "{}",
			update_name VARCHAR(80) DEFAULT "",
			target_name VARCHAR(80) DEFAULT "",
			ostree_hash VARCHAR(80) DEFAULT "",
			apps VARCHAR(2048) DEFAULT "",

			group_name_modified_at INT DEFAULT 0,

			name VARCHAR(80) GENERATED ALWAYS AS (
				COALESCE(labels ->> '$.name', "")
			) VIRTUAL,
			group_name VARCHAR(80) GENERATED ALWAYS AS (
				COALESCE(labels ->> '$.group', "")
			) VIRTUAL
		) WITHOUT ROWID;

		CREATE UNIQUE INDEX idx_device_name_unique ON devices(name) WHERE name != "";
		CREATE INDEX idx_device_name ON devices(name);
		CREATE INDEX idx_device_group ON devices(group_name);

		CREATE TABLE device_labels (
			label VARCHAR(20) NOT NULL PRIMARY KEY
		) WITHOUT ROWID;

		CREATE TRIGGER devices_after_update_labels AFTER UPDATE ON devices
		FOR EACH ROW
		WHEN OLD.labels != NEW.labels
		BEGIN
			INSERT OR IGNORE INTO device_labels(label)
			SELECT json_each.key FROM json_each(NEW.labels);
		END;

		CREATE TRIGGER devices_after_update_group_name AFTER UPDATE ON devices
		FOR EACH ROW
		WHEN OLD.group_name != NEW.group_name
		BEGIN
			UPDATE devices
			SET group_name_modified_at = unixepoch('now')
			WHERE uuid == NEW.uuid;
		END;

		CREATE TABLE users (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			username       TEXT NOT NULL UNIQUE,
			password       VARCHAR(128),
			email          TEXT,
			created_at     INT DEFAULT 0,
			deleted        BOOL DEFAULT 0,
			allowed_scopes TEXT DEFAULT "",

			auth_provider_data JSONB NOT NULL DEFAULT '{}'
		);

		CREATE TABLE tokens (
			public_id      INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id        INT,
			created_at     INT,
			expires_at     INT,
			description    VARCHAR(80),
			scopes         TEXT,
			value          VARCHAR(60) NOT NULL UNIQUE,

			FOREIGN KEY(user_id) REFERENCES user(id)
		);

		CREATE TABLE session (
			id             VARCHAR(64) NOT NULL PRIMARY KEY,
			user_id        INT,
			remote_ip      VARCHAR(39),
			created_at     INT,
			expires_at     INT,
			scopes         TEXT,
			FOREIGN KEY(user_id) REFERENCES user(id)
		) WITHOUT ROWID;
	`
	if _, err := db.Exec(sqlStmt); err != nil {
		return fmt.Errorf("unable to create devices db: %w", err)
	}
	return nil
}

type DbStmt struct {
	Stmt *sql.Stmt
}

type DbStmtInit interface {
	Init(db DbHandle) error
}
