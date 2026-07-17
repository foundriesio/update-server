// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"crypto/x509"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/foundriesio/update-server/storage"
)

func TestPkiInit(t *testing.T) {
	tmpDir := t.TempDir()

	cmd := PkiInitCmd{
		DnsName:       "example.com",
		Factory:       "example",
		TlsExpiryDays: 365,
		CaExpiryDays:  3650,
	}
	common := CommonArgs{DataDir: tmpDir}
	require.Nil(t, cmd.Run(common))

	fs, err := storage.NewFs(common.DataDir)
	require.Nil(t, err)

	// All expected files exist.
	for _, name := range []string{
		storage.CertsRootKeyFile, storage.CertsRootPemFile,
		storage.CertsTlsKeyFile, storage.CertsTlsPemFile,
		storage.CertsDeviceCaKeyFile, storage.CertsDeviceCaPemFile,
		storage.CertsCasPemFile,
	} {
		_, err := os.Stat(fs.Certs.FilePath(name))
		require.Nil(t, err, "missing file %s", name)
	}

	rootCrt, err := storage.LoadPemFile(fs.Certs.FilePath(storage.CertsRootPemFile), x509.ParseCertificate)
	require.Nil(t, err)
	require.True(t, rootCrt.IsCA)
	require.Equal(t, []string{"example"}, rootCrt.Subject.OrganizationalUnit)

	roots := x509.NewCertPool()
	roots.AddCert(rootCrt)

	// TLS cert chains to the root and carries the expected subject.
	tlsCrt, err := storage.LoadPemFile(fs.Certs.FilePath(storage.CertsTlsPemFile), x509.ParseCertificate)
	require.Nil(t, err)
	require.Equal(t, "example.com", tlsCrt.Subject.CommonName)
	require.Equal(t, []string{"example"}, tlsCrt.Subject.OrganizationalUnit)
	require.False(t, tlsCrt.IsCA)
	_, err = tlsCrt.Verify(x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSName:   "example.com",
	})
	require.Nil(t, err)

	// Device CA is a CA that chains to the root.
	deviceCrt, err := storage.LoadPemFile(fs.Certs.FilePath(storage.CertsDeviceCaPemFile), x509.ParseCertificate)
	require.Nil(t, err)
	require.True(t, deviceCrt.IsCA)
	_, err = deviceCrt.Verify(x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	})
	require.Nil(t, err)

	// cas.pem is the device CA.
	casBytes, err := fs.Certs.ReadFile(storage.CertsCasPemFile)
	require.Nil(t, err)
	deviceBytes, err := fs.Certs.ReadFile(storage.CertsDeviceCaPemFile)
	require.Nil(t, err)
	require.Equal(t, deviceBytes, casBytes)

	// Re-running refuses to overwrite the existing PKI.
	err = cmd.Run(common)
	require.NotNil(t, err)
	require.True(t, errors.Is(err, os.ErrExist))
}
