// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package pki

import (
	"github.com/spf13/cobra"
)

var PkiCmd = &cobra.Command{
	Use:   "pki",
	Short: "Show server PKI certificates",
	Long:  `Commands for viewing the Update server's PKI certificates.`,
}
