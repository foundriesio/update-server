// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package updates

import (
	"time"

	"github.com/foundriesio/update-server/cli/api"
	"github.com/foundriesio/update-server/cli/subcommands"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all updates",
	Long:  `List all updates`,
	Run: func(cmd *cobra.Command, args []string) {
		api := api.CtxGetApi(cmd.Context())
		listUpdates(api)
	},
}

func init() {
	UpdatesCmd.AddCommand(listCmd)
}

func listUpdates(api *api.Api) {
	allUpdates, err := api.Updates().List()
	cobra.CheckErr(err)

	t := subcommands.NewTableWriter([]string{"TAG", "NAME", "UPLOADED AT", "UPLOADED BY"})

	for tag, updates := range allUpdates {
		for _, update := range updates {
			t.AddRow(tag, update.Name, time.Unix(update.UploadedAt, 0).Format(time.RFC3339), update.UploadedBy)
		}
	}

	t.Render()
}
