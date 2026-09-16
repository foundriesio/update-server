// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"errors"
	"os"
	"slices"

	"github.com/foundriesio/update-server/storage"
)

func (s Storage) ReadCert(certName string) (string, error) {
	allowed := []string{
		storage.CertsCasPemFile,
		storage.CertsDeviceCaPemFile,
		storage.CertsRootPemFile,
		storage.CertsTlsPemFile,
	}
	if !slices.Contains(allowed, certName) {
		return "", nil // treat the error like a 404 not exist
	}
	buf, err := s.fs.Certs.ReadFile(certName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return string(buf), nil
}
