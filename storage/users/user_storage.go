// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package users

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/foundriesio/update-server/storage"
)

type User struct {
	h  Storage
	id int64

	Username string
	Password string
	Email    string

	CreatedAt int64
	Deleted   bool

	AllowedScopes Scopes

	AuthProviderData []byte
}

func (u User) Delete() error {
	u.Deleted = true
	if err := u.h.stmtTokenDeleteAll.run(u); err != nil {
		return fmt.Errorf("unable to delete user while deleting tokens: %w", err)
	}
	return u.Update("User deleted")
}

func (u User) Update(reason string) error {
	if err := u.h.stmtUserUpdate.run(u); err != nil {
		return err
	}
	u.h.fs.Audit.AppendEvent(u.id, reason)
	return nil
}

func (u User) GetAuditLog() (string, error) {
	return u.h.fs.Audit.ReadEvents(u.id)
}

func (u User) ApproveAuthorization(deviceCode string, tokenDescription string, scopes Scopes) error {
	if err := u.h.stmtOAuth2DeviceAuthAuthorize.run(u.id, deviceCode, tokenDescription, scopes.String()); err != nil {
		return err
	}
	u.h.fs.Audit.AppendEvent(u.id, "OAuth2 device authorized: "+tokenDescription)
	return nil
}

func (u User) DenyDeviceAuth(deviceCode string) error {
	if err := u.h.stmtOAuth2DeviceAuthDeny.run(deviceCode); err != nil {
		return err
	}
	u.h.fs.Audit.AppendEvent(u.id, "OAuth2 device denied")
	return nil
}

type Storage struct {
	db *storage.DbHandle
	fs *storage.FsHandle

	hmacSecret []byte

	stmtUserCreate    stmtUserCreate
	stmtUserGetById   stmtUserGetById
	stmtUserGetByName stmtUserGetByName
	stmtUserList      stmtUserList
	stmtUserUpdate    stmtUserUpdate

	stmtSessionCreate        stmtSessionCreate
	stmtSessionDelete        stmtSessionDelete
	stmtSessionDeleteExpired stmtSessionDeleteExpired
	stmtSessionGet           stmtSessionGet

	stmtTokenCreate        stmtTokenCreate
	stmtTokenDelete        stmtTokenDelete
	stmtTokenDeleteAll     stmtTokenDeleteAll
	stmtTokenDeleteExpired stmtTokenDeleteExpired
	stmtTokenList          stmtTokenList
	stmtTokenLookup        stmtTokenLookup

	// OAuth2 device-flow authorization
	stmtOAuth2DeviceAuthCreate          stmtOAuth2DeviceAuthCreate
	stmtOAuth2DeviceAuthGetByDeviceCode stmtOAuth2DeviceAuthGetByDeviceCode
	stmtOAuth2DeviceAuthGetByUserCode   stmtOAuth2DeviceAuthGetByUserCode
	stmtOAuth2DeviceAuthAuthorize       stmtOAuth2DeviceAuthAuthorize
	stmtOAuth2DeviceAuthDeny            stmtOAuth2DeviceAuthDeny
	stmtOAuth2DeviceAuthDelete          stmtOAuth2DeviceAuthDelete
	stmtOAuth2DeviceAuthDeleteExpired   stmtOAuth2DeviceAuthDeleteExpired
}

func NewStorage(db *storage.DbHandle, fs *storage.FsHandle) (*Storage, error) {
	hmacSecret, err := fs.Auth.GetHmacSecret()
	if err != nil {
		return nil, fmt.Errorf("unable to read HMAC secret for API tokens: %w", err)
	}
	handle := Storage{
		db:         db,
		fs:         fs,
		hmacSecret: hmacSecret,
	}

	if err := db.InitStmt(
		&handle.stmtUserCreate,
		&handle.stmtUserGetById,
		&handle.stmtUserGetByName,
		&handle.stmtUserList,
		&handle.stmtUserUpdate,
		&handle.stmtSessionCreate,
		&handle.stmtSessionDelete,
		&handle.stmtSessionDeleteExpired,
		&handle.stmtSessionGet,
		&handle.stmtTokenCreate,
		&handle.stmtTokenDelete,
		&handle.stmtTokenDeleteAll,
		&handle.stmtTokenDeleteExpired,
		&handle.stmtTokenList,
		&handle.stmtTokenLookup,
		&handle.stmtOAuth2DeviceAuthCreate,
		&handle.stmtOAuth2DeviceAuthGetByDeviceCode,
		&handle.stmtOAuth2DeviceAuthGetByUserCode,
		&handle.stmtOAuth2DeviceAuthAuthorize,
		&handle.stmtOAuth2DeviceAuthDeny,
		&handle.stmtOAuth2DeviceAuthDelete,
		&handle.stmtOAuth2DeviceAuthDeleteExpired,
	); err != nil {
		return nil, err
	}

	return &handle, nil
}

func (s Storage) RunGc() {
	now := time.Now().Unix()
	slog.Info("Running user token GC")
	if err := s.stmtTokenDeleteExpired.run(now); err != nil {
		slog.Error("Unable to run user token GC", "error", err)
	}

	slog.Info("Running user session GC")
	if err := s.stmtSessionDeleteExpired.run(now); err != nil {
		slog.Error("Unable to run user session GC", "error", err)
	}

	slog.Info("Running OAuth2 device-flow GC")
	if err := s.stmtOAuth2DeviceAuthDeleteExpired.run(now); err != nil {
		slog.Error("Unable to run OAuth2 device-flow GC", "error", err)
	}
}

func (s Storage) Create(u *User) error {
	err := s.stmtUserCreate.run(u)
	if err == nil {
		u.h = s
		s.fs.Audit.AppendEvent(u.id, "User created")
	}
	return err
}

func (s Storage) Upsert(username, email string, scopes Scopes) (*User, error) {
	u, err := s.stmtUserGetByName.run(username)
	switch err {
	case sql.ErrNoRows:
		u = &User{
			Username:      username,
			Email:         email,
			AllowedScopes: scopes,
		}
		return u, s.Create(u)
	case nil:
		u.h = s
	}
	return u, err
}

func (s Storage) Get(username string) (*User, error) {
	u, err := s.stmtUserGetByName.run(username)
	switch err {
	case sql.ErrNoRows:
		return nil, nil
	case nil:
		u.h = s
	}
	return u, err
}

func (s Storage) List() ([]User, error) {
	users, err := s.stmtUserList.run()
	if err == nil {
		for i := range users {
			users[i].h = s
		}
	}
	return users, err
}

type stmtUserCreate storage.DbStmt

func (s *stmtUserCreate) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("userCreate", `
		INSERT INTO users (username, password, email, created_at, deleted, allowed_scopes, auth_provider_data)
		VALUES (?, ?, ?, ?, ?, ?, jsonb(?))`,
	)
	return
}

func (s *stmtUserCreate) run(u *User) error {
	if u.CreatedAt == 0 {
		u.CreatedAt = time.Now().Unix()
	}
	if u.AuthProviderData == nil {
		u.AuthProviderData = []byte("{}")
	}
	result, err := s.Stmt.Exec(
		u.Username,
		u.Password,
		u.Email,
		u.CreatedAt,
		u.Deleted,
		u.AllowedScopes.String(),
		u.AuthProviderData,
	)
	if err != nil {
		return err
	} else if id, err := result.LastInsertId(); err != nil {
		return err
	} else {
		u.id = id
	}
	return nil
}

type stmtUserGetById storage.DbStmt

func (s *stmtUserGetById) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("userGetId", `
		SELECT id, username, password, email, created_at, allowed_scopes, json_extract(auth_provider_data, '$')
		FROM users
		WHERE id = ? and deleted = false`,
	)
	return
}

func (s *stmtUserGetById) run(id int64) (*User, error) {
	u := User{}
	scopeStr := ""
	err := s.Stmt.QueryRow(id).Scan(
		&u.id,
		&u.Username,
		&u.Password,
		&u.Email,
		&u.CreatedAt,
		&scopeStr,
		&u.AuthProviderData,
	)
	if err == nil {
		u.AllowedScopes, err = ScopesFromString(scopeStr)
		if err != nil {
			return nil, fmt.Errorf("unable to parse scopes: %w", err)
		}
	}
	return &u, err
}

type stmtUserGetByName storage.DbStmt

func (s *stmtUserGetByName) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("userGet", `
		SELECT id, username, password, email, created_at, allowed_scopes, json_extract(auth_provider_data, '$')
		FROM users
		WHERE username = ? AND deleted = false`,
	)
	return
}

func (s *stmtUserGetByName) run(username string) (*User, error) {
	u := User{}
	var scopesStr string
	err := s.Stmt.QueryRow(username).Scan(
		&u.id,
		&u.Username,
		&u.Password,
		&u.Email,
		&u.CreatedAt,
		&scopesStr,
		&u.AuthProviderData,
	)
	if err == nil {
		u.AllowedScopes, err = ScopesFromString(scopesStr)
		if err != nil {
			return nil, fmt.Errorf("unable to parse scopes: %w", err)
		}
	}
	return &u, err
}

type stmtUserList storage.DbStmt

func (s *stmtUserList) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("userList", `
		SELECT id, username, password, email, created_at, deleted, allowed_scopes
		FROM users
		WHERE deleted = false`,
	)
	return
}

func (s *stmtUserList) run() ([]User, error) {
	var users []User
	rows, err := s.Stmt.Query()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error("stmtUserList: failed to close rows", "error", err)
		}
	}()

	for rows.Next() {
		var u User
		var scopesStr string
		err := rows.Scan(
			&u.id,
			&u.Username,
			&u.Password,
			&u.Email,
			&u.CreatedAt,
			&u.Deleted,
			&scopesStr,
		)
		if err != nil {
			return nil, err
		}
		u.AllowedScopes, err = ScopesFromString(scopesStr)
		if err != nil {
			return nil, fmt.Errorf("unable to parse scopes: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

type stmtUserUpdate storage.DbStmt

func (s *stmtUserUpdate) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("userUpdate", `
		UPDATE users
		SET username = ?, password = ?, email = ?, allowed_scopes = ?, deleted = ?, auth_provider_data = jsonb(?)
		WHERE id = ?`,
	)
	return
}

func (s *stmtUserUpdate) run(u User) error {
	if u.AuthProviderData == nil {
		u.AuthProviderData = []byte("{}")
	}
	_, err := s.Stmt.Exec(u.Username, u.Password, u.Email, u.AllowedScopes.String(), u.Deleted, u.AuthProviderData, u.id)
	return err
}
