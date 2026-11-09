// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"crypto/x509"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/foundriesio/update-server/server/ui/api"
	"github.com/foundriesio/update-server/storage"
)

func TestPkiInit(t *testing.T) {
	tmpDir := t.TempDir()

	cmd := PkiInitCmd{
		DnsName:       "example.com",
		OU:            "example",
		TlsExpiryDays: 365,
		CaExpiryDays:  3650,
	}
	common := CommonArgs{DataDir: tmpDir}
	require.NoError(t, cmd.Run(common))

	fs, err := storage.NewFs(common.DataDir)
	require.NoError(t, err)

	// All expected files exist.
	for _, name := range []string{
		storage.CertsRootKeyFile, storage.CertsRootPemFile,
		storage.CertsTlsKeyFile, storage.CertsTlsPemFile,
		storage.CertsDeviceCaKeyFile, storage.CertsDeviceCaPemFile,
		storage.CertsCasPemFile,
	} {
		_, err := os.Stat(fs.Certs.FilePath(name))
		require.NoError(t, err, "missing file %s", name)
	}

	rootCrt, err := storage.LoadPemFile(fs.Certs.FilePath(storage.CertsRootPemFile), x509.ParseCertificate)
	require.NoError(t, err)
	require.True(t, rootCrt.IsCA)
	require.Equal(t, []string{"example"}, rootCrt.Subject.OrganizationalUnit)

	roots := x509.NewCertPool()
	roots.AddCert(rootCrt)

	// TLS cert chains to the root and carries the expected subject.
	tlsCrt, err := storage.LoadPemFile(fs.Certs.FilePath(storage.CertsTlsPemFile), x509.ParseCertificate)
	require.NoError(t, err)
	require.Equal(t, "example.com", tlsCrt.Subject.CommonName)
	require.Equal(t, []string{"example"}, tlsCrt.Subject.OrganizationalUnit)
	require.False(t, tlsCrt.IsCA)
	_, err = tlsCrt.Verify(x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSName:   "example.com",
	})
	require.NoError(t, err)

	// Device CA is a CA that chains to the root.
	deviceCrt, err := storage.LoadPemFile(fs.Certs.FilePath(storage.CertsDeviceCaPemFile), x509.ParseCertificate)
	require.NoError(t, err)
	require.True(t, deviceCrt.IsCA)
	_, err = deviceCrt.Verify(x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	})
	require.NoError(t, err)

	// cas.pem is the device CA.
	casBytes, err := fs.Certs.ReadFile(storage.CertsCasPemFile)
	require.NoError(t, err)
	deviceBytes, err := fs.Certs.ReadFile(storage.CertsDeviceCaPemFile)
	require.NoError(t, err)
	require.Equal(t, deviceBytes, casBytes)

	// Re-running refuses to overwrite the existing PKI.
	err = cmd.Run(common)
	require.ErrorIs(t, err, os.ErrExist)
}

func TestPkiInitWithIp(t *testing.T) {
	tmpDir := t.TempDir()

	cmd := PkiInitCmd{
		DnsName:       "example.com",
		IpAddress:     "192.0.2.10",
		OU:            "example",
		TlsExpiryDays: 365,
		CaExpiryDays:  3650,
	}
	common := CommonArgs{DataDir: tmpDir}
	require.NoError(t, cmd.Run(common))

	fs, err := storage.NewFs(common.DataDir)
	require.NoError(t, err)
	rootCrt, err := storage.LoadPemFile(fs.Certs.FilePath(storage.CertsRootPemFile), x509.ParseCertificate)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	roots.AddCert(rootCrt)

	// The TLS cert is valid for both the name and the address.
	tlsCrt, err := storage.LoadPemFile(fs.Certs.FilePath(storage.CertsTlsPemFile), x509.ParseCertificate)
	require.NoError(t, err)
	for _, host := range []string{"example.com", "192.0.2.10"} {
		_, err = tlsCrt.Verify(x509.VerifyOptions{
			Roots:     roots,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			DNSName:   host,
		})
		require.NoError(t, err, host)
	}

	ca, err := api.LoadDeviceCa(fs, ":8443")
	require.NoError(t, err)
	require.Equal(t, "https://192.0.2.10:8443", ca.DgUrl)
}

func TestPkiInitInvalidIp(t *testing.T) {
	cmd := PkiInitCmd{DnsName: "example.com", IpAddress: "not-an-ip", OU: "example", TlsExpiryDays: 365, CaExpiryDays: 3650}
	require.ErrorContains(t, cmd.Run(CommonArgs{DataDir: t.TempDir()}), "invalid IP address")
}
