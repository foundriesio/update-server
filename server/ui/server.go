// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package ui

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"time"

	"github.com/foundriesio/update-server/auth"
	"github.com/foundriesio/update-server/server"
	apiHandlers "github.com/foundriesio/update-server/server/ui/api"
	"github.com/foundriesio/update-server/server/ui/daemons"
	webHandlers "github.com/foundriesio/update-server/server/ui/web"
	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/api"
	"github.com/foundriesio/update-server/storage/users"
)

const serverName = "rest-api"

type daemon interface {
	Start()
	Shutdown()
}

func NewServer(ctx context.Context, db *storage.DbHandle, fs *storage.FsHandle, bindAddr string) (server.Server, error) {
	tuf, err := storage.LoadTuf(fs)
	if err != nil {
		return nil, fmt.Errorf("TUF not initialized: run 'tuf-init' first: %w", err)
	}
	strg, err := api.NewStorage(db, fs, tuf)
	if err != nil {
		return nil, fmt.Errorf("failed to load %s storage: %w", serverName, err)
	}
	users, err := users.NewStorage(db, fs)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize users storage: %w", err)
	}
	e := server.NewEchoServer()

	provider, err := auth.NewProvider(e, db, fs, users)
	if err != nil {
		return nil, err
	}
	slog.Info("Using authentication provider", "name", provider.Name())

	daemons := daemons.New(ctx, strg, users)

	srv := server.NewServer(ctx, e, serverName, bindAddr, nil)
	e.Use(auth.CsrfCheck)
	apiHandlers.RegisterHandlers(e, strg, tuf, serverBaseURI(fs), provider)
	webHandlers.RegisterHandlers(e, users, provider)
	return &apiServer{server: srv, daemons: daemons}, nil
}

// serverBaseURI returns "https://<dns>" from the TLS cert, or empty string if cert is absent.
func serverBaseURI(fs *storage.FsHandle) string {
	certPEM, err := fs.Certs.ReadFile(storage.CertsTlsPemFile)
	if err != nil {
		return ""
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return ""
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil || len(cert.DNSNames) == 0 {
		return ""
	}
	return "https://" + cert.DNSNames[0]
}

type apiServer struct {
	server  server.Server
	daemons daemon
}

func (s apiServer) Start(quit chan error) {
	s.daemons.Start()
	s.server.Start(quit)
}

func (s apiServer) Shutdown(timeout time.Duration) {
	s.daemons.Shutdown()
	s.server.Shutdown(timeout)
}

func (s apiServer) GetAddress() string {
	return s.server.GetAddress()
}

func (s apiServer) GetDnsName() string {
	return s.server.GetDnsName()
}
