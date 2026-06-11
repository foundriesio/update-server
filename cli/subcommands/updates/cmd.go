// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package updates

import (
	"github.com/spf13/cobra"
)

var UpdatesCmd = &cobra.Command{
	Use:   "updates",
	Short: "Manage updates",
	Long:  `Commands for managing updates in the Update server`,
}
