// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

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

	caCrt, err := loadCert(c.CaCert)
	if err != nil {
		return err
	}

	caKey, err := loadKey(c.CaKey)
	if err != nil {
		return err
	}

	csr, err := loadCsr(fs.Certs.FilePath(storage.CertsTlsCsrFile))
	if err != nil {
		return err
	}
	tlsKey, err := loadKey(fs.Certs.FilePath(storage.CertsTlsKeyFile))
	if err != nil {
		return err
	}

	max := big.NewInt(0).Exp(big.NewInt(2), big.NewInt(160), nil)
	serial, err := rand.Int(rand.Reader, max)
	if err != nil {
		return fmt.Errorf("error generating certificate serial number: %w", err)
	}

	crtTemplate := &x509.Certificate{
		Subject:      csr.Subject,
		Issuer:       caCrt.Subject,
		SerialNumber: serial,
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(0, 0, c.ExpiryDays),

		IsCA:        false,
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    csr.DNSNames,
	}

	certDer, err := x509.CreateCertificate(rand.Reader, crtTemplate, caCrt, &tlsKey.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("error signing TLS cert: %w", err)
	}
	certPem := pem.EncodeToMemory(
		&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: certDer,
		},
	)

	if err := fs.Certs.WriteFile(storage.CertsTlsPemFile, certPem); err != nil {
		return fmt.Errorf("unable to store TLS certificate: %w", err)
	}

	return nil
}

func loadCert(path string) (*x509.Certificate, error) {
	cert, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("unable to read cert: %w", err)
	}

	first, rest := pem.Decode(cert)
	if first == nil || len(strings.TrimSpace(string(rest))) > 0 {
		return nil, fmt.Errorf("malformed PEM data for %s", path)
	}

	return x509.ParseCertificate(first.Bytes)
}

func loadCsr(path string) (*x509.CertificateRequest, error) {
	csr, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("unable to read csr: %w", err)
	}

	first, rest := pem.Decode(csr)
	if first == nil || len(strings.TrimSpace(string(rest))) > 0 {
		return nil, fmt.Errorf("malformed PEM data for %s", path)
	}

	return x509.ParseCertificateRequest(first.Bytes)
}

func loadKey(path string) (*ecdsa.PrivateKey, error) {
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("unable to read pkey: %w", err)
	}

	first, rest := pem.Decode(key)
	if first == nil || len(strings.TrimSpace(string(rest))) > 0 {
		return nil, fmt.Errorf("malformed PEM data for %s", path)
	}

	return x509.ParseECPrivateKey(first.Bytes)
}
