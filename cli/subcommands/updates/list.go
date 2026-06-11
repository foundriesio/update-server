// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package updates

import (
	"github.com/foundriesio/update-server/cli/api"
	"github.com/foundriesio/update-server/cli/subcommands"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all updates",
	Long:  `List all CI and production updates`,
	Run: func(cmd *cobra.Command, args []string) {
		api := api.CtxGetApi(cmd.Context())
		listUpdates(api)
	},
}

func init() {
	UpdatesCmd.AddCommand(listCmd)
}

func listUpdates(api *api.Api) {
	ciUpdates, err := api.Updates("ci").List()
	cobra.CheckErr(err)

	prodUpdates, err := api.Updates("prod").List()
	cobra.CheckErr(err)

	t := subcommands.NewTableWriter([]string{"TYPE", "TAG", "NAME"})

	for tag, names := range ciUpdates {
		for _, name := range names {
			t.AddRow("ci", tag, name)
		}
	}

	for tag, names := range prodUpdates {
		for _, name := range names {
			t.AddRow("prod", tag, name)
		}
	}

	t.Render()
}
