// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package users

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"

	"github.com/foundriesio/update-server/storage"
)

func (s Storage) hashSessionID(id string) (string, error) {
	key, err := s.genTokenKey(id)
	if err != nil {
		return "", err
	}
	hasher := hmac.New(sha256.New, key)
	if _, err := hasher.Write([]byte(id)); err != nil {
		return "", fmt.Errorf("unable to hash session id: %w", err)
	}
	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

func (s Storage) GetBySession(id string) (*User, error) {
	hashed, err := s.hashSessionID(id)
	if err != nil {
		return nil, err
	}
	sess, err := s.stmtSessionGet.run(hashed)
	if err != nil {
		return nil, err
	} else if sess == nil {
		return nil, nil
	}
	if sess.ExpiresAt < time.Now().Unix() {
		return nil, nil
	}
	u, err := s.stmtUserGetById.run(sess.UserID)
	if u != nil {
		u.h = s
		u.AllowedScopes = sess.Scopes & u.AllowedScopes
	}

	return u, err
}

func (u User) CreateSession(remoteIP string, expires int64, scopes Scopes) (string, error) {
	if scopes&u.AllowedScopes != scopes {
		return "", fmt.Errorf("requested scopes %s exceed allowed scopes %s", scopes.String(), u.AllowedScopes.String())
	}
	idStr := rand.Text()
	hashed, err := u.h.hashSessionID(idStr)
	if err != nil {
		return "", fmt.Errorf("unable to hash session id: %w", err)
	}
	if err := u.h.stmtSessionCreate.run(u, hashed, remoteIP, time.Now().Unix(), expires, scopes); err != nil {
		return "", fmt.Errorf("unable to create session: %w", err)
	}

	msg := fmt.Sprintf("Session created (ip=%s, expires=%d, scopes=%s)", remoteIP, expires, scopes)
	u.h.fs.Audit.AppendEvent(u.id, msg)
	return idStr, nil
}

func (u User) DeleteSession(id string) error {
	hashed, err := u.h.hashSessionID(id)
	if err != nil {
		return fmt.Errorf("unable to hash session id: %w", err)
	}
	if err := u.h.stmtSessionDelete.run(hashed); err != nil {
		return fmt.Errorf("unable to delete session: %w", err)
	}
	msg := fmt.Sprintf("Session deleted id=%s", id)
	u.h.fs.Audit.AppendEvent(u.id, msg)
	return nil
}

type session struct {
	UserID    int64
	RemoteIP  string
	ExpiresAt int64
	Scopes    Scopes
}

type stmtSessionCreate storage.DbStmt

func (s *stmtSessionCreate) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("sessionCreate", `
		INSERT INTO session (id, user_id, remote_ip, created_at, expires_at, scopes)
		VALUES (?, ?, ?, ?, ?, ?)`,
	)
	return
}

func (s *stmtSessionCreate) run(u User, id, remoteIP string, created, expires int64, scopes Scopes) error {
	_, err := s.Stmt.Exec(
		id,
		u.id,
		remoteIP,
		created,
		expires,
		scopes.String(),
	)
	return err
}

type stmtSessionDelete storage.DbStmt

func (s *stmtSessionDelete) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("sessionDelete", `
		DELETE FROM session
		WHERE id = ?`,
	)
	return
}

func (s *stmtSessionDelete) run(id string) error {
	_, err := s.Stmt.Exec(id)
	return err
}

type stmtSessionDeleteExpired storage.DbStmt

func (s *stmtSessionDeleteExpired) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("sessionDeleteExpired", `
		DELETE FROM session
		WHERE expires_at < ?`,
	)
	return
}

func (s *stmtSessionDeleteExpired) run(before int64) error {
	_, err := s.Stmt.Exec(before)
	return err
}

type stmtSessionGet storage.DbStmt

func (s *stmtSessionGet) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("sessionGet", `
		SELECT user_id, expires_at, scopes
		FROM session
		WHERE id = ?`,
	)
	return
}

func (s *stmtSessionGet) run(id string) (*session, error) {
	var sess session
	var scopesStr string
	err := s.Stmt.QueryRow(id).Scan(
		&sess.UserID,
		&sess.ExpiresAt,
		&scopesStr,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	} else {
		sess.Scopes, err = ScopesFromString(scopesStr)
		if err != nil {
			return nil, fmt.Errorf("unable to parse scopes: %w", err)
		}
	}
	return &sess, nil
}
