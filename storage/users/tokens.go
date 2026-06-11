// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package users

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/foundriesio/update-server/storage"
	"golang.org/x/crypto/hkdf"
)

type Token struct {
	PublicID    int64
	CreatedAt   int64
	ExpiresAt   int64
	Description string
	Scopes      Scopes
	Value       string
}

func (s Storage) genTokenKey(token string) ([]byte, error) {
	if len(token) < 17 {
		return nil, fmt.Errorf("token too short to derive key")
	}
	salt := []byte(token[3:17])
	key := make([]byte, 32)
	if _, err := io.ReadFull(hkdf.New(sha256.New, s.hmacSecret, salt, nil), key); err != nil {
		return nil, fmt.Errorf("unable to derive encryption key for token: %w", err)
	}
	return key, nil
}

func (s Storage) GetByToken(token string) (*User, error) {
	key, err := s.genTokenKey(token)
	if err != nil {
		return nil, err
	}

	hasher := hmac.New(sha256.New, key)
	if _, err := hasher.Write([]byte(token)); err != nil {
		return nil, fmt.Errorf("unable to hash token value: %w", err)
	}
	hashed := fmt.Sprintf("%x", hasher.Sum(nil))
	t, userID, err := s.stmtTokenLookup.run(hashed)
	if err != nil {
		return nil, err
	} else if t == nil {
		return nil, nil
	}

	if t.ExpiresAt < time.Now().Unix() {
		return nil, nil
	}
	u, err := s.stmtUserGetById.run(userID)
	if u != nil {
		u.h = s
		u.AllowedScopes = t.Scopes & u.AllowedScopes
	}
	return u, err
}

func (u User) GenerateToken(description string, expires int64, scopes Scopes) (*Token, error) {
	if scopes&u.AllowedScopes != scopes {
		return nil, fmt.Errorf("requested scopes %s exceed allowed scopes %s", scopes.String(), u.AllowedScopes.String())
	}

	value := rand.Text()
	key, err := u.h.genTokenKey(value)
	if err != nil {
		return nil, err
	}

	hasher := hmac.New(sha256.New, key)
	if _, err := hasher.Write([]byte(value)); err != nil {
		return nil, fmt.Errorf("unable to hash token value: %w", err)
	}
	hashed := fmt.Sprintf("%x", hasher.Sum(nil))

	t := Token{
		CreatedAt:   time.Now().Unix(),
		ExpiresAt:   expires,
		Description: description,
		Scopes:      scopes,
		Value:       hashed,
	}

	if err := u.h.stmtTokenCreate.run(u, &t); err != nil {
		return nil, err
	}
	msg := fmt.Sprintf("Token created (id=%d, expires=%d, scopes=%s)", t.PublicID, expires, scopes)
	u.h.fs.Audit.AppendEvent(u.id, msg)
	t.Value = value
	return &t, nil
}

func (u User) DeleteToken(id int64) error {
	if err := u.h.stmtTokenDelete.run(u, id); err != nil {
		return err
	}
	msg := fmt.Sprintf("Token deleted id=%d", id)
	u.h.fs.Audit.AppendEvent(u.id, msg)
	return nil
}

func (u User) ListTokens() ([]Token, error) {
	return u.h.stmtTokenList.run(u)
}

type stmtTokenCreate storage.DbStmt

func (s *stmtTokenCreate) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("tokenCreate", `
		INSERT INTO tokens (user_id, created_at, expires_at, description, scopes, value)
		VALUES (?, ?, ?, ?, ?, ?)`,
	)
	return
}

func (s *stmtTokenCreate) run(u User, t *Token) error {
	result, err := s.Stmt.Exec(
		u.id,
		t.CreatedAt,
		t.ExpiresAt,
		t.Description,
		t.Scopes.String(),
		t.Value,
	)
	if err != nil {
		return err
	} else if id, err := result.LastInsertId(); err != nil {
		return err
	} else {
		t.PublicID = id
	}
	return err
}

type stmtTokenDelete storage.DbStmt

func (s *stmtTokenDelete) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("tokenDelete", `
		DELETE FROM tokens
		WHERE user_id = ? and public_id = ?`,
	)
	return
}

func (s *stmtTokenDelete) run(u User, id int64) error {
	_, err := s.Stmt.Exec(u.id, id)
	return err
}

type stmtTokenDeleteAll storage.DbStmt

func (s *stmtTokenDeleteAll) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("tokenDeleteAll", `
		DELETE FROM tokens
		WHERE user_id = ?`,
	)
	return
}

func (s *stmtTokenDeleteAll) run(u User) error {
	_, err := s.Stmt.Exec(u.id)
	return err
}

type stmtTokenDeleteExpired storage.DbStmt

func (s *stmtTokenDeleteExpired) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("tokenDeleteExpired", `
		DELETE FROM tokens
		WHERE expires_at < ?`,
	)
	return
}

func (s *stmtTokenDeleteExpired) run(before int64) error {
	_, err := s.Stmt.Exec(before)
	return err
}

type stmtTokenList storage.DbStmt

func (s *stmtTokenList) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("tokenList", `
		SELECT public_id, created_at, expires_at, description, scopes
		FROM tokens
		WHERE user_id = ?
		ORDER BY created_at ASC`,
	)
	return
}

func (s *stmtTokenList) run(u User) ([]Token, error) {
	var tokens []Token
	rows, err := s.Stmt.Query(u.id)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error("stmtTokenList: failed to close rows", "error", err)
		}
	}()

	for rows.Next() {
		var scopesStr string
		var t Token
		err := rows.Scan(
			&t.PublicID,
			&t.CreatedAt,
			&t.ExpiresAt,
			&t.Description,
			&scopesStr,
		)
		if err != nil {
			return nil, err
		}
		t.Scopes, err = ScopesFromString(scopesStr)
		if err != nil {
			return nil, fmt.Errorf("unable to parse scopes: %w", err)
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

type stmtTokenLookup storage.DbStmt

func (s *stmtTokenLookup) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("tokenLookup", `
		SELECT user_id, public_id, created_at, expires_at, scopes
		FROM tokens
		WHERE value = ?`,
	)
	return
}

func (s *stmtTokenLookup) run(value string) (*Token, int64, error) {
	var t Token
	var userID int64
	var scopesStr string
	err := s.Stmt.QueryRow(value).Scan(
		&userID,
		&t.PublicID,
		&t.CreatedAt,
		&t.ExpiresAt,
		&scopesStr,
	)
	if err == sql.ErrNoRows {
		return nil, 0, nil
	} else if err != nil {
		return nil, 0, err
	} else {
		t.Scopes, err = ScopesFromString(scopesStr)
		if err != nil {
			return nil, 0, fmt.Errorf("unable to parse scopes: %w", err)
		}
	}
	return &t, userID, nil
}
