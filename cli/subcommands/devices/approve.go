// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package devices

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/foundriesio/update-server/cli/api"
)

var approveCmd = &cobra.Command{
	Use:   "approve <user-code>",
	Short: "Approve a pending device registration code",
	Long: `Approve the user code printed by fio-device-register, as the server's
activation page does.

This context's token needs the devices:create scope. The device gets a
token with only that scope.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		description, _ := cmd.Flags().GetString("description")
		api := api.CtxGetApi(cmd.Context())
		cobra.CheckErr(api.Devices().ApproveDevice(args[0], description))
		fmt.Printf("Approved %s\n", args[0])
	},
}

func init() {
	approveCmd.Flags().StringP("description", "d", "", "Description stored with the issued token")
	DevicesCmd.AddCommand(approveCmd)
}
