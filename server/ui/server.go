// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package ui

import (
	"fmt"
	"time"

	"github.com/foundriesio/update-server/auth"
	"github.com/foundriesio/update-server/context"
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
	log := context.CtxGetLog(ctx)
	strg, err := api.NewStorage(db, fs)
	if err != nil {
		return nil, fmt.Errorf("failed to load %s storage: %w", serverName, err)
	}
	users, err := users.NewStorage(db, fs)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize users storage: %w", err)
	}
	e := server.NewEchoServer()

	brandingData, err := fs.Config.ReadBrandingConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to read branding config: %w", err)
	}
	branding := webHandlers.LoadBranding(brandingData)
	pageBuilder := webHandlers.NewPageBuilder(branding)

	provider, err := auth.NewProvider(e, db, fs, users, pageBuilder)
	if err != nil {
		return nil, err
	}
	log.Info("Using authentication provider", "name", provider.Name())

	daemons := daemons.New(ctx, strg, users)

	ca, err := apiHandlers.LoadDeviceCa(fs)
	if err != nil {
		return nil, fmt.Errorf("failed to load device CA: %w", err)
	} else if ca != nil {
		log.Info("Device CA is configured, enabling device registration endpoint")
	}

	srv := server.NewServer(ctx, e, serverName, bindAddr, nil)
	e.Use(auth.CsrfCheck)
	apiHandlers.RegisterHandlers(e, ca, strg, provider)
	webHandlers.RegisterHandlers(e, users, provider, branding, fs.Config.BrandingDir(), pageBuilder)
	return &apiServer{server: srv, daemons: daemons}, nil
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
