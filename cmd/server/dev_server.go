// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"fmt"
	"net"
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
	fmt.Println("# PKI initialized")
	fmt.Println("device-gateway TLS DNS name:", displayDnsName)
	fmt.Println("Devices must connect to this server with this name.")
	if ip, err := outboundIP(); err == nil {
		fmt.Println("If DNS resolution fails, edit /etc/hosts on devices with:")
		fmt.Printf("| %s\t%s\n", ip, dnsName)
	} else {
		fmt.Println("If DNS resolution fails, edit /etc/hosts on your devices.")
	}
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

// outboundIP returns the local address this host would use to reach the
// network, by opening a UDP "connection" (no packets are sent) to a
// well-known external address and reading back the chosen source IP.
func outboundIP() (net.IP, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, err
	}
	defer conn.Close() //nolint:errcheck
	return conn.LocalAddr().(*net.UDPAddr).IP, nil
}
