// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
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
	require.Nil(t, csr.Run(common))

	fs, err := storage.NewFs(common.DataDir)
	require.Nil(t, err)
	// Create a root CA
	caKeyFile, caFile := createSelfSignedRoot(t, fs)

	sign := CsrSignCmd{
		CaKey:  caKeyFile,
		CaCert: caFile,
	}
	require.Nil(t, sign.Run(common))

	cert, err := loadCert(fs.Certs.FilePath(storage.CertsTlsPemFile))
	require.Nil(t, err)
	require.Equal(t, "example.com", cert.Subject.CommonName)

	// fail second run because we require a new directory (so we don't accidentally overwrite a key)
	err = csr.Run(common)
	require.NotNil(t, err)
	require.True(t, errors.Is(err, os.ErrExist))
}

func createSelfSignedRoot(t *testing.T, fs *storage.FsHandle) (string, string) {
	caKeyFile := fs.Certs.FilePath(storage.CertsTlsKeyFile) // reuse the key we already generated
	key, err := loadKey(caKeyFile)
	require.Nil(t, err)

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
	require.Nil(t, err)
	caPem := pem.EncodeToMemory(
		&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: caDer,
		},
	)
	caFile := fs.Certs.FilePath("ca.pem")
	require.Nil(t, os.WriteFile(caFile, caPem, 0o640))
	return caKeyFile, caFile
}
