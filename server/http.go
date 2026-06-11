// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package server

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/random"

	"github.com/foundriesio/update-server/context"
)

type Server interface {
	Start(quit chan error)
	Shutdown(timeout time.Duration)
	GetAddress() string
	GetDnsName() string
}

type server struct {
	context context.Context
	name    string
	echo    *echo.Echo
	server  *http.Server
}

func NewServer(ctx context.Context, echo *echo.Echo, name string, bindAddr string, tlsConfig *tls.Config) Server {
	log := context.CtxGetLog(ctx).With("server", name)
	ctx = context.CtxWithLog(ctx, log)
	srv := &http.Server{
		Addr:        bindAddr,
		BaseContext: func(net.Listener) context.Context { return ctx },
		ConnContext: adjustConnContext,
		TLSConfig:   tlsConfig,
	}
	// We cannot push request context, but at least make it JSON, show the server name and error file line.
	echo.StdLogger = context.StdLogAdapter(log, true)
	return &server{context: ctx, name: name, echo: echo, server: srv}
}

func (s server) Start(quit chan error) {
	log := context.CtxGetLog(s.context)
	go func() {
		if err := s.echo.StartServer(s.server); err != nil && err != http.ErrServerClosed {
			log.Error("failed to start server", "error", err)
			quit <- fmt.Errorf("failed to start server %s: %w", s.name, err)
		}
	}()
	go func() {
		// Echo locks a mutex immediately at the Start call, and releases after port binding is done.
		// GetAddress will be locked for that duration; but we need to give it a tiny favor to start.
		time.Sleep(time.Millisecond * 2)
		if addr := s.GetAddress(); addr != "" {
			args := []any{"addr", addr}
			if s.echo.TLSListener != nil {
				args = append(args, "dns_name", s.GetDnsName())
			}
			log.Info("server started", args...)
		}
	}()
}

func (s server) Shutdown(timeout time.Duration) {
	ctx, cancel := context.WithTimeout(s.context, timeout)
	defer cancel()
	if err := s.echo.Shutdown(ctx); err != nil {
		log := context.CtxGetLog(s.context)
		log.Error("error stopping server", "error", err)
	}
}

func (s server) GetAddress() (ret string) {
	// ListenerAddr waits for the server to start before returning
	if addr := s.echo.TLSListenerAddr(); addr != nil {
		// Addr can be nil when server fails to start
		ret = addr.String()
	} else if addr := s.echo.ListenerAddr(); addr != nil {
		ret = addr.String()
	}
	return
}

func (s server) GetDnsName() (ret string) {
	log := context.CtxGetLog(s.context)
	if s.server.TLSConfig == nil || len(s.server.TLSConfig.Certificates) == 0 {
		log.Error("programming error", "error", errTlsNotConfigured)
	} else if cert := s.server.TLSConfig.Certificates[0].Leaf; cert == nil {
		log.Error("programming error", "error", errTlsLeafCert)
	} else if len(cert.DNSNames) > 0 {
		ret = cert.DNSNames[0]
	}
	return
}

func adjustConnContext(ctx context.Context, conn net.Conn) context.Context {
	cid := random.String(10) // No need for uuid, save some space
	log := context.CtxGetLog(ctx).With("conn_id", cid)
	// There is nothing meaningful to log before the TLS connection
	return context.CtxWithLog(ctx, log)
}

var (
	errTlsNotConfigured = errors.New("server TLS not configured properly")
	errTlsLeafCert      = errors.New("since Golang 1.23, LoadX509KeyPair always sets TLS leaf cert")
)
