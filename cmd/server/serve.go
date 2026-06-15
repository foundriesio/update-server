// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/foundriesio/update-server/server"
	"github.com/foundriesio/update-server/server/gateway"
	"github.com/foundriesio/update-server/server/ui"
	"github.com/foundriesio/update-server/storage"
)

type ServeCmd struct {
	startedCb func(uiAddress, gatewayAddress string)

	UiAddr      string `default:":8080"`
	GatewayAddr string `default:":8443"`
}

func (c *ServeCmd) Run(args CommonArgs) error {
	fs, err := storage.NewFs(args.DataDir)
	if err != nil {
		return fmt.Errorf("failed to load filesystem: %w", err)
	}
	db, err := storage.NewDb(fs.Config.DbFile())
	if err != nil {
		return fmt.Errorf("failed to load database: %w", err)
	}
	uiServer, err := ui.NewServer(args.ctx, db, fs, c.UiAddr)
	if err != nil {
		return err
	}
	gtwServer, err := gateway.NewServer(args.ctx, db, fs, c.GatewayAddr)
	if err != nil {
		return err
	}

	quitErr := make(chan error, 2)
	uiServer.Start(quitErr)
	gtwServer.Start(quitErr)

	if c.startedCb != nil {
		// Testing code, see serve_test.go
		time.Sleep(time.Millisecond * 2)
		c.startedCb(uiServer.GetAddress(), gtwServer.GetAddress())
	}

	// setup channel to gracefully terminate server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	select {
	case err = <-quitErr:
	case <-quit:
		break
	}

	var wg sync.WaitGroup
	wg.Add(2)
	for _, srv := range []server.Server{uiServer, gtwServer} {
		go func() {
			srv.Shutdown(time.Minute)
			wg.Done()
		}()
	}
	wg.Wait()

	return err
}
