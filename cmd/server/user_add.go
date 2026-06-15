// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/foundriesio/update-server/auth"
	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/users"
)

type UserAddCmd struct {
	Username      string   `arg:"required" help:"Username for the new user"`
	Password      string   `arg:"" help:"Password for the new user (read from stdin if not provided)"`
	AllowedScopes []string `arg:"" help:"Roles to assign to the new user"`
}

func (c UserAddCmd) Run(args CommonArgs) error {
	if c.Password == "" {
		fmt.Print("Enter password: ")
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		c.Password = strings.TrimSpace(string(pw))
		if c.Password == "" {
			return fmt.Errorf("password cannot be empty")
		}
	}

	fs, err := storage.NewFs(args.DataDir)
	if err != nil {
		return err
	}

	cfg, err := fs.Auth.GetAuthConfig()
	if err != nil {
		return fmt.Errorf("failed to load auth config: %w", err)
	}
	if cfg.Type != "local" {
		return fmt.Errorf("user-add is only supported with local auth. Configured auth is: %s", cfg.Type)
	}

	if len(c.AllowedScopes) == 0 {
		c.AllowedScopes = cfg.NewUserDefaultScopes
	}
	scopes, err := users.ScopesFromSlice(c.AllowedScopes)
	if err != nil {
		return fmt.Errorf("invalid scopes: %w", err)
	}

	db, err := storage.NewDb(fs.Config.DbFile())
	if err != nil {
		return fmt.Errorf("failed to load database: %w", err)
	}

	userStorage, err := users.NewStorage(db, fs)
	if err != nil {
		return fmt.Errorf("failed to initialize user storage: %w", err)
	}

	if u, err := userStorage.Get(c.Username); err == nil && u != nil {
		return fmt.Errorf("user %q already exists", c.Username)
	}

	password, err := auth.PasswordHash(c.Password)
	if err != nil {
		return err
	}

	u := &users.User{
		Username:      c.Username,
		Password:      password,
		AllowedScopes: scopes,
	}

	return userStorage.Create(u)
}
