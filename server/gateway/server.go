// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package gateway

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"time"

	"github.com/foundriesio/update-server/context"
	"github.com/foundriesio/update-server/server"
	storage "github.com/foundriesio/update-server/storage/gateway"
)

const serverName = "gateway-api"

func NewServer(ctx context.Context, db *storage.DbHandle, fs *storage.FsHandle, bindAddr string) (server.Server, error) {
	tlsCfg, err := loadTlsConfig(fs)
	if err != nil {
		return nil, fmt.Errorf("failed to load %s TLS config: %w", serverName, err)
	}
	strg, err := storage.NewStorage(db, fs)
	if err != nil {
		return nil, fmt.Errorf("failed to load %s storage: %w", serverName, err)
	}

	e := server.NewEchoServer()
	srv := server.NewServer(ctx, e, serverName, bindAddr, tlsCfg)

	_, port, err := net.SplitHostPort(bindAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid bind address %q: %w", bindAddr, err)
	}
	url := "https://" + net.JoinHostPort(srv.GetDnsName(), port)

	RegisterHandlers(e, strg, url)
	return srv, nil
}

func loadTlsConfig(fs *storage.FsHandle) (*tls.Config, error) {
	caPool, err := loadCas(fs)
	if err != nil {
		return nil, fmt.Errorf("failed to load gateway cert: %w", err)
	}
	kp, err := loadTlsKeyPair(fs)
	if err != nil {
		return nil, fmt.Errorf("failed to load gateway key: %w", err)
	}

	leaf, err := x509.ParseCertificate(kp.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse TLS certificate: %w", err)
	}
	if time.Now().After(leaf.NotAfter) {
		return nil, fmt.Errorf("TLS certificate expired on %s", leaf.NotAfter)
	}

	cfg := &tls.Config{
		Certificates: []tls.Certificate{kp},
		ClientAuth:   tls.VerifyClientCertIfGiven,
		MinVersion:   tls.VersionTLS12,
		ClientCAs:    caPool,
	}
	return cfg, nil
}

func loadCas(fs *storage.FsHandle) (*x509.CertPool, error) {
	bytes, err := fs.Certs.ReadFile(storage.CertsCasPemFile)
	if err != nil {
		return nil, fmt.Errorf("unable to read CAs file: %w", err)
	}

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(bytes)
	return caPool, nil
}

func loadTlsKeyPair(fs *storage.FsHandle) (tls.Certificate, error) {
	keyFile := fs.Certs.FilePath(storage.CertsTlsKeyFile)
	certFile := fs.Certs.FilePath(storage.CertsTlsPemFile)
	return tls.LoadX509KeyPair(certFile, keyFile)
}
