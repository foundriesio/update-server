// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
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

	priv, csrBytes, err := buildCsr(c.DnsName, c.Factory)
	if err != nil {
		return err
	}

	privPem, err := marshalKeyPem(priv)
	if err != nil {
		return err
	}
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
