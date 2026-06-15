// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package updates

import (
	"fmt"

	"github.com/foundriesio/update-server/cli/api"
	"github.com/foundriesio/update-server/cli/subcommands"
	"github.com/spf13/cobra"
)

var showCmd = &cobra.Command{
	Use:   "show <tag> <update-name>",
	Short: "Show rollouts for an update",
	Long:  `Display all rollouts for a specific update`,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		api := api.CtxGetApi(cmd.Context())
		showUpdate(api.Updates(), args[0], args[1])
		return nil
	},
}

func init() {
	UpdatesCmd.AddCommand(showCmd)
}

func showUpdate(updates api.UpdatesApi, tag, updateName string) {
	rollouts, err := updates.Get(tag, updateName)
	cobra.CheckErr(err)

	if len(rollouts) == 0 {
		fmt.Printf("No rollouts found for update %s/%s\n", tag, updateName)
		return
	}

	fmt.Printf("Update: %s\n", updateName)
	fmt.Printf("Tag: %s\n\n", tag)

	t := subcommands.NewTableWriter([]string{"ROLLOUT NAME"})

	for _, rollout := range rollouts {
		t.AddRow(rollout)
	}

	t.Render()
}
