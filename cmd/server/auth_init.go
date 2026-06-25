// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"encoding/json"

	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/users"
)

type AuthInitCmd struct {
	Test  bool `help:"Initialize auth with test config: full access for everyone"`
	Local bool `help:"Initialize auth with local username/password (relaxed rules, dev only)"`
}

func (c AuthInitCmd) Run(args CommonArgs) error {
	if fs, err := storage.NewFs(args.DataDir); err != nil {
		return err
	} else if err = fs.Auth.InitHmacSecret(); err != nil {
		return err
	} else if c.Local {
		cfg := storage.AuthConfig{
			Type:                 "local",
			NewUserDefaultScopes: users.ScopesAvailable(),
			Config: json.RawMessage(`{"MinPasswordLength":0,"PasswordHistory":0,"PasswordAgeDays":0,` +
				`"PasswordComplexityRules":{"RequireUppercase":false,"RequireLowercase":false,` +
				`"RequireDigit":false,"RequireSpecialChar":""}}`),
		}
		return fs.Auth.SaveAuthConfig(cfg)
	} else if c.Test {
		cfg := storage.AuthConfig{
			Type:                 "noauth",
			NewUserDefaultScopes: users.ScopesAvailable(),
		}
		return fs.Auth.SaveAuthConfig(cfg)
	} else {
		return nil
	}
}
