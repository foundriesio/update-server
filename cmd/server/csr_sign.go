// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"crypto/x509"
	"fmt"

	"github.com/foundriesio/update-server/storage"
)

type CsrSignCmd struct {
	CaKey      string `arg:"required" help:"Factory rook PKI key"`
	CaCert     string `arg:"required" help:"Factory rook PKI cert"`
	ExpiryDays int    `default:"365" help:"TLS certificate validity in days"`
}

func (c CsrSignCmd) Run(args CommonArgs) error {
	fs, err := storage.NewFs(args.DataDir)
	if err != nil {
		return err
	}

	caCrt, err := storage.LoadPemFile(c.CaCert, x509.ParseCertificate)
	if err != nil {
		return err
	}

	caKey, err := storage.LoadPemFile(c.CaKey, x509.ParseECPrivateKey)
	if err != nil {
		return err
	}

	csr, err := storage.LoadPemFile(fs.Certs.FilePath(storage.CertsTlsCsrFile), x509.ParseCertificateRequest)
	if err != nil {
		return err
	}
	tlsKey, err := storage.LoadPemFile(fs.Certs.FilePath(storage.CertsTlsKeyFile), x509.ParseECPrivateKey)
	if err != nil {
		return err
	}

	serial, err := newSerial()
	if err != nil {
		return err
	}

	crtTemplate := tlsCertTemplate(csr, caCrt, serial, c.ExpiryDays)

	certPem, err := signCert(crtTemplate, caCrt, &tlsKey.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("error signing TLS cert: %w", err)
	}

	if err := fs.Certs.WriteFile(storage.CertsTlsPemFile, certPem); err != nil {
		return fmt.Errorf("unable to store TLS certificate: %w", err)
	}

	return nil
}
