// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"fmt"

	"github.com/foundriesio/update-server/storage"
)

type TufInitCmd struct{}

func (c *TufInitCmd) Run(args CommonArgs) error {
	fs, err := storage.NewFs(args.DataDir)
	if err != nil {
		return fmt.Errorf("failed to load filesystem: %w", err)
	}
	if _, err = storage.InitTuf(fs); err != nil {
		return fmt.Errorf("failed to initialize TUF: %w", err)
	}
	fmt.Println("TUF initialized successfully")
	return nil
}
