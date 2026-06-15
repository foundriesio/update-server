// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package devices

import (
	"github.com/foundriesio/update-server/cli/api"
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <uuid>",
	Short: "Delete a device",
	Long:  `Delete a device from the server`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		api := api.CtxGetApi(cmd.Context())
		cobra.CheckErr(api.Devices().Delete(args[0]))
		return nil
	},
}

func init() {
	DevicesCmd.AddCommand(deleteCmd)
}
