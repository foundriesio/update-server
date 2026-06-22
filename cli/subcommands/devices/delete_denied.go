// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package devices

import (
	"github.com/foundriesio/update-server/cli/api"
	"github.com/spf13/cobra"
)

var deleteDeniedCmd = &cobra.Command{
	Use:   "delete-denied <uuid>",
	Short: "Remove a device from the denied list",
	Long: `Remove a device from the denied list so it can access the backend again.
Only the server record is affected; device data (configs, update events,
apps states) removed when the device was denied is not recovered.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		a := api.CtxGetApi(cmd.Context())
		cobra.CheckErr(a.Devices().DeleteDenied(args[0]))
		return nil
	},
}

func init() {
	DevicesCmd.AddCommand(deleteDeniedCmd)
}
