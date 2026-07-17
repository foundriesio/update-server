// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"time"

	"github.com/foundriesio/update-server/storage"
)

type PkiInitCmd struct {
	DnsName       string `arg:"required" help:"DNS host name devices address this gateway with"`
	Factory       string `arg:"required"`
	TlsExpiryDays int    `default:"365" help:"TLS certificate validity in days"`
	CaExpiryDays  int    `default:"7300" help:"Root and device CA certificate validity in days"`
}

func (c PkiInitCmd) Run(args CommonArgs) error {
	fs, err := storage.NewFs(args.DataDir)
	if err != nil {
		return err
	}
	if err = fs.Certs.AssertCleanPki(); err != nil {
		return err
	}

	// 1. Root CA
	rootKey, err := generateEcKey()
	if err != nil {
		return err
	}
	serial, err := newSerial()
	if err != nil {
		return err
	}
	rootTemplate := &x509.Certificate{
		Subject: pkix.Name{
			CommonName:         c.Factory + "-root",
			OrganizationalUnit: []string{c.Factory},
		},
		SerialNumber:          serial,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, 0, c.CaExpiryDays),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	rootPem, err := signCert(rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		return fmt.Errorf("error creating root CA: %w", err)
	}
	// signCert produced a self-signed cert; parse it back so it can be used
	// to issue child certificates.
	rootCrt, err := storage.PemBytesToObject(rootPem, x509.ParseCertificate)
	if err != nil {
		return fmt.Errorf("error parsing root CA: %w", err)
	}
	rootKeyPem, err := marshalKeyPem(rootKey)
	if err != nil {
		return err
	}
	if err := fs.Certs.WriteFile(storage.CertsRootKeyFile, rootKeyPem); err != nil {
		return fmt.Errorf("unable to store root CA key: %w", err)
	}
	if err := fs.Certs.WriteFile(storage.CertsRootPemFile, rootPem); err != nil {
		return fmt.Errorf("unable to store root CA cert: %w", err)
	}

	// 2. TLS keypair signed by the root CA (reusing the create-csr/sign-csr path)
	tlsKey, csrBytes, err := buildCsr(c.DnsName, c.Factory)
	if err != nil {
		return err
	}
	csr, err := x509.ParseCertificateRequest(csrBytes)
	if err != nil {
		return fmt.Errorf("error parsing generated CSR: %w", err)
	}
	tlsSerial, err := newSerial()
	if err != nil {
		return err
	}
	tlsTemplate := tlsCertTemplate(csr, rootCrt, tlsSerial, c.TlsExpiryDays)
	tlsPem, err := signCert(tlsTemplate, rootCrt, &tlsKey.PublicKey, rootKey)
	if err != nil {
		return fmt.Errorf("error signing TLS cert: %w", err)
	}
	tlsKeyPem, err := marshalKeyPem(tlsKey)
	if err != nil {
		return err
	}
	if err := fs.Certs.WriteFile(storage.CertsTlsKeyFile, tlsKeyPem); err != nil {
		return fmt.Errorf("unable to store TLS key: %w", err)
	}
	if err := fs.Certs.WriteFile(storage.CertsTlsPemFile, tlsPem); err != nil {
		return fmt.Errorf("unable to store TLS cert: %w", err)
	}

	// 3. Device signing CA signed by the root CA
	deviceKey, err := generateEcKey()
	if err != nil {
		return err
	}
	deviceSerial, err := newSerial()
	if err != nil {
		return err
	}
	deviceTemplate := &x509.Certificate{
		Subject: pkix.Name{
			CommonName:         c.Factory + "-device-ca",
			OrganizationalUnit: []string{c.Factory},
		},
		Issuer:                rootCrt.Subject,
		SerialNumber:          deviceSerial,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, 0, c.CaExpiryDays),
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	devicePem, err := signCert(deviceTemplate, rootCrt, &deviceKey.PublicKey, rootKey)
	if err != nil {
		return fmt.Errorf("error signing device CA: %w", err)
	}
	deviceKeyPem, err := marshalKeyPem(deviceKey)
	if err != nil {
		return err
	}
	if err := fs.Certs.WriteFile(storage.CertsDeviceCaKeyFile, deviceKeyPem); err != nil {
		return fmt.Errorf("unable to store device CA key: %w", err)
	}
	if err := fs.Certs.WriteFile(storage.CertsDeviceCaPemFile, devicePem); err != nil {
		return fmt.Errorf("unable to store device CA cert: %w", err)
	}

	// 4. Trust the device CA for device mTLS
	if err := fs.Certs.WriteFile(storage.CertsCasPemFile, devicePem); err != nil {
		return fmt.Errorf("unable to store cas.pem: %w", err)
	}

	return nil
}
