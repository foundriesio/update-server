// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package devices

import "github.com/spf13/cobra"

var DevicesCmd = &cobra.Command{
	Use:   "devices",
	Short: "Manage devices",
	Long:  `Commands for managing devices in the Foundries Update server`,
}
