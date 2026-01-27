// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package users

import (
	"crypto/rand"
	"database/sql"

	"github.com/foundriesio/update-server/storage"
)

// OAuth2DeviceAuth represents an OAuth2 device-flow authorization request
type OAuth2DeviceAuth struct {
	DeviceCode       string
	UserCode         string
	ExpiresAt        int64
	TokenExpires     int64  // What time the issued token should expire
	TokenDescription string // Description to be stored with the issued token
	Scopes           string // The scopes to be assigned to the token
	UserID           *int64
	Authorized       bool
	Denied           bool
}

func (s Storage) CreateDeviceAuth(expiresAt, tokenExpires int64, scopes string) (string, string, error) {
	deviceCode := rand.Text()
	userCode := rand.Text()
	userCode = userCode[:4] + "-" + userCode[4:8]
	return deviceCode, userCode, s.stmtOAuth2DeviceAuthCreate.run(deviceCode, userCode, expiresAt, tokenExpires, scopes)
}

func (s Storage) GetDeviceAuthByDeviceCode(deviceCode string) (*OAuth2DeviceAuth, error) {
	return s.stmtOAuth2DeviceAuthGetByDeviceCode.run(deviceCode)
}

func (s Storage) GetDeviceAuthByUserCode(userCode string) (*OAuth2DeviceAuth, error) {
	return s.stmtOAuth2DeviceAuthGetByUserCode.run(userCode)
}

func (s Storage) DeleteExpiredDeviceAuth(before int64) error {
	return s.stmtOAuth2DeviceAuthDeleteExpired.run(before)
}

func (s Storage) DeleteDeviceAuth(deviceCode string) error {
	return s.stmtOAuth2DeviceAuthDelete.run(deviceCode)
}

type stmtOAuth2DeviceAuthCreate storage.DbStmt

func (s *stmtOAuth2DeviceAuthCreate) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("oauth2DeviceAuthCreate", `
		INSERT INTO oauth2_device_flow (device_code, user_code, expires_at, token_expires, token_description, scopes)
		VALUES (?, ?, ?, ?, "", ?)`,
	)
	return
}

func (s *stmtOAuth2DeviceAuthCreate) run(deviceCode, userCode string, expiresAt, tokenExpires int64, scopes string) error {
	_, err := s.Stmt.Exec(deviceCode, userCode, expiresAt, tokenExpires, scopes)
	return err
}

type stmtOAuth2DeviceAuthGetByDeviceCode storage.DbStmt

func (s *stmtOAuth2DeviceAuthGetByDeviceCode) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("oauth2DeviceAuthGetByDeviceCode", `
		SELECT device_code, user_code, expires_at, token_expires, token_description, scopes, user_id, authorized, denied
		FROM oauth2_device_flow
		WHERE device_code = ?`,
	)
	return
}

func (s *stmtOAuth2DeviceAuthGetByDeviceCode) run(deviceCode string) (*OAuth2DeviceAuth, error) {
	auth := &OAuth2DeviceAuth{}
	var userID sql.NullInt64

	err := s.Stmt.QueryRow(deviceCode).Scan(
		&auth.DeviceCode,
		&auth.UserCode,
		&auth.ExpiresAt,
		&auth.TokenExpires,
		&auth.TokenDescription,
		&auth.Scopes,
		&userID,
		&auth.Authorized,
		&auth.Denied,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if userID.Valid {
		auth.UserID = &userID.Int64
	}

	return auth, nil
}

type stmtOAuth2DeviceAuthGetByUserCode storage.DbStmt

func (s *stmtOAuth2DeviceAuthGetByUserCode) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("oauth2DeviceAuthGetByUserCode", `
		SELECT device_code, user_code, expires_at, token_expires, token_description, scopes, user_id, authorized, denied
		FROM oauth2_device_flow
		WHERE user_code = ?`,
	)
	return
}

func (s *stmtOAuth2DeviceAuthGetByUserCode) run(userCode string) (*OAuth2DeviceAuth, error) {
	auth := &OAuth2DeviceAuth{}
	var userID sql.NullInt64

	err := s.Stmt.QueryRow(userCode).Scan(
		&auth.DeviceCode,
		&auth.UserCode,
		&auth.ExpiresAt,
		&auth.TokenExpires,
		&auth.TokenDescription,
		&auth.Scopes,
		&userID,
		&auth.Authorized,
		&auth.Denied,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if userID.Valid {
		auth.UserID = &userID.Int64
	}

	return auth, nil
}

type stmtOAuth2DeviceAuthAuthorize storage.DbStmt

func (s *stmtOAuth2DeviceAuthAuthorize) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("oauth2DeviceAuthAuthorize", `
		UPDATE oauth2_device_flow
		SET user_id = ?, authorized = 1, token_description = ?, scopes = ?
		WHERE device_code = ? AND authorized = 0 AND denied = 0`,
	)
	return
}

func (s *stmtOAuth2DeviceAuthAuthorize) run(userID int64, deviceCode, tokenDescription, scopes string) error {
	_, err := s.Stmt.Exec(userID, tokenDescription, scopes, deviceCode)
	return err
}

type stmtOAuth2DeviceAuthDeny storage.DbStmt

func (s *stmtOAuth2DeviceAuthDeny) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("oauth2DeviceAuthDeny", `
		UPDATE oauth2_device_flow
		SET denied = 1
		WHERE device_code = ? AND authorized = 0 AND denied = 0`,
	)
	return
}

func (s *stmtOAuth2DeviceAuthDeny) run(deviceCode string) error {
	_, err := s.Stmt.Exec(deviceCode)
	return err
}

type stmtOAuth2DeviceAuthDelete storage.DbStmt

func (s *stmtOAuth2DeviceAuthDelete) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("oauth2DeviceAuthDelete", `
		DELETE FROM oauth2_device_flow
		WHERE device_code = ?`,
	)
	return
}

func (s *stmtOAuth2DeviceAuthDelete) run(deviceCode string) error {
	_, err := s.Stmt.Exec(deviceCode)
	return err
}

type stmtOAuth2DeviceAuthDeleteExpired storage.DbStmt

func (s *stmtOAuth2DeviceAuthDeleteExpired) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("oauth2DeviceAuthDeleteExpired", `
		DELETE FROM oauth2_device_flow
		WHERE expires_at < ?`,
	)
	return
}

func (s *stmtOAuth2DeviceAuthDeleteExpired) run(before int64) error {
	_, err := s.Stmt.Exec(before)
	return err
}
