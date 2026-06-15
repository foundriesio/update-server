// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"

	"github.com/foundriesio/update-server/storage"
)

type CsrCmd struct {
	DnsName string `arg:"required" help:"DNS host name devices address this gateway with"`
	Factory string `arg:"required"`
}

func (c CsrCmd) Run(args CommonArgs) error {
	fs, err := storage.NewFs(args.DataDir)
	if err != nil {
		return err
	}
	if err = fs.Certs.AssertCleanTls(); err != nil {
		return err
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("unexpected error generating private key for CSR: %w", err)
	}

	subj := pkix.Name{
		CommonName:         c.DnsName,
		OrganizationalUnit: []string{c.Factory},
	}

	template := x509.CertificateRequest{
		Subject:            subj,
		SignatureAlgorithm: x509.ECDSAWithSHA256,
		DNSNames:           []string{c.DnsName},
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &template, priv)
	if err != nil {
		return fmt.Errorf("unexpected error creating CSR: %w", err)
	}

	privDer, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return fmt.Errorf("unexpected error encoding private key: %w", err)
	}
	privPem := pem.EncodeToMemory(
		&pem.Block{
			Type:  "EC PRIVATE KEY",
			Bytes: privDer,
		},
	)
	if err := fs.Certs.WriteFile(storage.CertsTlsKeyFile, privPem); err != nil {
		return fmt.Errorf("unable to store TLS private key file: %w", err)
	}

	csrPem := pem.EncodeToMemory(
		&pem.Block{
			Type:  "CERTIFICATE REQUEST",
			Bytes: csrBytes,
		},
	)
	if err := fs.Certs.WriteFile(storage.CertsTlsCsrFile, csrPem); err != nil {
		return fmt.Errorf("unable to store TLS CSR file: %w", err)
	}
	fmt.Println(string(csrPem))
	return nil
}
