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
	"math/big"
	"time"
)

// generateEcKey creates a new EC P-256 private key.
func generateEcKey() (*ecdsa.PrivateKey, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("unexpected error generating private key: %w", err)
	}
	return priv, nil
}

// marshalKeyPem PEM-encodes an EC private key.
func marshalKeyPem(key *ecdsa.PrivateKey) ([]byte, error) {
	privDer, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("unexpected error encoding private key: %w", err)
	}
	return pem.EncodeToMemory(
		&pem.Block{
			Type:  "EC PRIVATE KEY",
			Bytes: privDer,
		},
	), nil
}

// newSerial generates a random 160-bit certificate serial number.
func newSerial() (*big.Int, error) {
	max := big.NewInt(0).Exp(big.NewInt(2), big.NewInt(160), nil)
	serial, err := rand.Int(rand.Reader, max)
	if err != nil {
		return nil, fmt.Errorf("error generating certificate serial number: %w", err)
	}
	return serial, nil
}

// signCert creates a certificate from template, signed by parent using
// parentKey for the given public key, and returns the PEM-encoded certificate.
func signCert(template, parent *x509.Certificate, pub, parentKey any) ([]byte, error) {
	certDer, err := x509.CreateCertificate(rand.Reader, template, parent, pub, parentKey)
	if err != nil {
		return nil, fmt.Errorf("error signing certificate: %w", err)
	}
	return pem.EncodeToMemory(
		&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: certDer,
		},
	), nil
}

// tlsCertTemplate builds the x509 template for a server TLS certificate derived
// from a CSR and issued by caCrt.
func tlsCertTemplate(csr *x509.CertificateRequest, caCrt *x509.Certificate, serial *big.Int, expiryDays int) *x509.Certificate {
	return &x509.Certificate{
		Subject:      csr.Subject,
		Issuer:       caCrt.Subject,
		SerialNumber: serial,
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(0, 0, expiryDays),

		IsCA:        false,
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    csr.DNSNames,
	}
}

// buildCsr generates a new EC key and a TLS certificate signing request for the
// given DNS name and factory. It returns the key and the DER-encoded CSR bytes.
func buildCsr(dnsName, factory string) (*ecdsa.PrivateKey, []byte, error) {
	priv, err := generateEcKey()
	if err != nil {
		return nil, nil, err
	}

	template := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:         dnsName,
			OrganizationalUnit: []string{factory},
		},
		SignatureAlgorithm: x509.ECDSAWithSHA256,
		DNSNames:           []string{dnsName},
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &template, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("unexpected error creating CSR: %w", err)
	}
	return priv, csrBytes, nil
}
