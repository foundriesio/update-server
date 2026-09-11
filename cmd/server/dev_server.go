// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"fmt"
	"os"

	"golang.org/x/term"

	"github.com/foundriesio/update-server/storage"
)

// DevServerInitCmd bootstraps a brand new data directory in one step for
// local development: a self-signed PKI (pki-init), local username/password
// authentication with an admin user (auth-init + user-add), and TUF root
// metadata (tuf-init).
type DevServerInitCmd struct {
	DnsName       string `help:"DNS host name devices address this gateway with (default: system host name)"`
	Factory       string `default:"acme" help:"Factory name"`
	AdminPassword string `arg:"--admin-password" default:"admin" help:"Password for the local admin user"`
}

func (c DevServerInitCmd) Run(args CommonArgs) error {
	fs, err := storage.NewFs(args.DataDir)
	if err != nil {
		return err
	}
	if err := fs.Certs.AssertCleanPki(); err != nil {
		return fmt.Errorf("data dir %s is already initialized: %w", args.DataDir, err)
	}

	dnsName := c.DnsName
	if dnsName == "" {
		dnsName, err = os.Hostname()
		if err != nil {
			return fmt.Errorf("unable to determine system host name: %w", err)
		}
	}

	pkiInit := PkiInitCmd{DnsName: dnsName, Factory: c.Factory, TlsExpiryDays: 365, CaExpiryDays: 7300}
	if err := pkiInit.Run(args); err != nil {
		return fmt.Errorf("pki-init: %w", err)
	}
	displayDnsName := dnsName
	displayPassword := c.AdminPassword
	if term.IsTerminal(int(os.Stdout.Fd())) {
		displayDnsName = "\033[1m" + dnsName + "\033[0m"
		displayPassword = "\033[1m" + c.AdminPassword + "\033[0m"
	}
	fmt.Println("# PKI initialized with device-gateway TLS DNS name:", displayDnsName)
	fmt.Println("Devices must connect to this server with this name.")
	fmt.Println("Edit /etc/hosts on the device if necessary.")
	fmt.Println()

	if err := (AuthInitCmd{Local: true}).Run(args); err != nil {
		return fmt.Errorf("auth-init: %w", err)
	}
	fmt.Println("# Local authentication initialized")
	fmt.Println()

	if err := fs.Tuf.InitTuf(); err != nil {
		return err
	}
	fmt.Println("# TUF metadata initialized")
	fmt.Println()

	userAdd := UserAddCmd{Username: "admin", Password: c.AdminPassword}
	if err := userAdd.Run(args); err != nil {
		return fmt.Errorf("failed to create admin user: %w", err)
	}
	fmt.Println("# `admin` user created with password:", displayPassword)

	return nil
}
