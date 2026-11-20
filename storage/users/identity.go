// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package users

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/foundriesio/update-server/storage"
)

var ErrIdentityDeleted = errors.New("SSO account is deleted")

func (s Storage) UpsertIdentity(provider, subject, username, email string, scopes Scopes) (*User, error) {
	if strings.TrimSpace(provider) == "" || strings.TrimSpace(subject) == "" {
		return nil, errors.New("SSO provider and subject are required")
	}
	if username == "" {
		username = provider
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			slog.Error("unable to roll back SSO account transaction", "error", err)
		}
	}()

	var id int64
	for suffix := 1; ; suffix++ {
		candidate := username
		if suffix > 1 {
			candidate = fmt.Sprintf("%s-%d", username, suffix)
		}
		// The identity constraint arbitrates concurrent first logins. Names
		// only choose a free label and must never select an existing account.
		//nolint:unqueryvet // Retry username collisions; identity creation must remain atomic.
		err = tx.QueryRow(`
			INSERT INTO users (username, password, email, created_at, allowed_scopes, auth_provider, auth_subject)
			VALUES (?, '', ?, ?, ?, ?, ?)
			ON CONFLICT(auth_provider, auth_subject) DO NOTHING
			RETURNING id`, candidate, email, time.Now().Unix(), scopes.String(), provider, subject).Scan(&id)
		if storage.IsDbError(err, storage.ErrDbConstraintUnique) {
			continue
		}
		break
	}
	created := err == nil
	if errors.Is(err, sql.ErrNoRows) {
		var deleted bool
		err = tx.QueryRow(`SELECT id, deleted FROM users WHERE auth_provider = ? AND auth_subject = ?`,
			provider, subject).Scan(&id, &deleted)
		if err != nil {
			return nil, err
		}
		if deleted {
			return nil, ErrIdentityDeleted
		}
		_, err = tx.Exec(`UPDATE users SET email = ? WHERE id = ?`, email, id)
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if created {
		s.fs.Audit.AppendEvent(id, "User created")
	}
	return s.GetByID(id)
}
