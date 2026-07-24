// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package updates

import (
	"github.com/foundriesio/update-server/cli/api"
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <tag> <update-name>",
	Short: "Delete an update",
	Long:  `Delete an update from the server. Fails if devices are still assigned to it.`,
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		api := api.CtxGetApi(cmd.Context())
		cobra.CheckErr(api.Updates().Delete(args[0], args[1]))
	},
}

func init() {
	UpdatesCmd.AddCommand(deleteCmd)
}
