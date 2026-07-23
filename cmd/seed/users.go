// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"fmt"
	"log"

	"github.com/foundriesio/update-server/auth"
	"github.com/foundriesio/update-server/storage/users"
)

// seedUsers creates a couple of extra local-auth users with varied scopes,
// plus an API token for one of them, so the Users and Settings pages have
// more to show than the single bootstrap admin. Idempotent: skips any
// username that already exists.
func seedUsers(userStorage *users.Storage) error {
	seedAccounts := []struct {
		username string
		scopes   users.Scopes
	}{
		{"seed-viewer", users.ScopeDevicesR | users.ScopeUpdatesR},
		{"seed-operator", users.ScopeDevicesRU | users.ScopeUpdatesRU | users.ScopeUsersR},
	}

	for _, acct := range seedAccounts {
		if existing, err := userStorage.Get(acct.username); err != nil {
			return fmt.Errorf("Get(%s): %w", acct.username, err)
		} else if existing != nil {
			log.Printf("skip  user %s (already exists)", acct.username)
			continue
		}

		password, err := auth.PasswordHash("seed-password")
		if err != nil {
			return fmt.Errorf("PasswordHash(%s): %w", acct.username, err)
		}
		u := &users.User{
			Username:      acct.username,
			Password:      password,
			AllowedScopes: acct.scopes,
		}
		if err := userStorage.Create(u); err != nil {
			return fmt.Errorf("Create(%s): %w", acct.username, err)
		}
		log.Printf("create user %s", acct.username)

		if acct.username == "seed-operator" {
			const oneYear = 365 * 24 * 60 * 60
			if _, err := u.GenerateToken("seed API token", u.CreatedAt+oneYear, users.ScopeDevicesR); err != nil {
				return fmt.Errorf("GenerateToken(%s): %w", acct.username, err)
			}
		}
	}
	return nil
}
