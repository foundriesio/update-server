// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type CertsFsHandle struct {
	osRootHandle
}

func (s CertsFsHandle) ReadFile(name string) ([]byte, error) {
	content, err := s.root.ReadFile(name)
	if err != nil {
		err = fmt.Errorf("error reading file %s: %w", name, err)
	}
	return content, err
}

func (s CertsFsHandle) WriteFile(name string, content []byte) error {
	fileAccess := defaultFileAccess
	if strings.HasSuffix(name, ".key") {
		fileAccess = secureFileAccess
	}
	if err := s.writeFile(name, string(content), fileAccess); err != nil {
		return fmt.Errorf("error writing file %s: %w", name, err)
	}
	return nil
}

func (s CertsFsHandle) AssertCleanTls() error {
	return s.assertClean([]string{
		CertsTlsCsrFile, CertsTlsKeyFile, CertsTlsPemFile,
	})
}

func (s CertsFsHandle) AssertCleanPki() error {
	return s.assertClean([]string{
		CertsTlsCsrFile, CertsTlsKeyFile, CertsTlsPemFile,
		CertsRootKeyFile, CertsRootPemFile,
		CertsDeviceCaKeyFile, CertsDeviceCaPemFile,
		CertsCasPemFile,
	})
}

func (s CertsFsHandle) assertClean(names []string) error {
	for _, name := range names {
		if _, err := s.root.Stat(name); err == nil {
			return fmt.Errorf("a certificate file %s already exists: %w", name, os.ErrExist)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to check if a certificate file %s exists: %w", name, err)
		}
	}
	return nil
}
