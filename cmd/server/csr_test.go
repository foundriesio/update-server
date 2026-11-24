// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/foundriesio/update-server/storage"
)

func TestCsr(t *testing.T) {
	tmpDir := t.TempDir()

	csr := CsrCmd{
		DnsName: "example.com",
		Factory: "example",
	}

	common := CommonArgs{
		DataDir: tmpDir,
	}
	require.NoError(t, csr.Run(common))

	fs, err := storage.NewFs(common.DataDir)
	require.NoError(t, err)
	// Create a root CA
	caKeyFile, caFile := createSelfSignedRoot(t, fs)

	sign := CsrSignCmd{
		CaKey:  caKeyFile,
		CaCert: caFile,
	}
	require.NoError(t, sign.Run(common))

	cert, err := storage.LoadCertPem(fs.Certs, storage.CertsTlsPemFile, x509.ParseCertificate)
	require.NoError(t, err)
	require.Equal(t, "example.com", cert.Subject.CommonName)

	// fail second run because we require a new directory (so we don't accidentally overwrite a key)
	err = csr.Run(common)
	require.Error(t, err)
	require.ErrorIs(t, err, os.ErrExist)
}

func createSelfSignedRoot(t *testing.T, fs *storage.FsHandle) (string, string) {
	t.Helper()
	// Reuse the generated key, but exercise external CA paths for signing.
	keyPem, err := fs.Certs.ReadFile(storage.CertsTlsKeyFile)
	require.NoError(t, err)
	key, err := storage.PemBytesToObject(keyPem, x509.ParseECPrivateKey)
	require.NoError(t, err)
	caDir := t.TempDir()
	caKeyFile := filepath.Join(caDir, "ca.key")
	require.NoError(t, os.WriteFile(caKeyFile, keyPem, 0o600))

	ca := &x509.Certificate{
		SerialNumber: big.NewInt(2019),
		Subject: pkix.Name{
			Organization: []string{"example"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caDer, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	require.NoError(t, err)
	caPem := pem.EncodeToMemory(
		&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: caDer,
		},
	)
	caFile := filepath.Join(caDir, "ca.pem")
	require.NoError(t, os.WriteFile(caFile, caPem, 0o640))
	return caKeyFile, caFile
}
