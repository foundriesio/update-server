// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package configs

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/foundriesio/dg-satellite/cli/api"
)

var listGroups = &cobra.Command{
	Use:   "groups",
	Short: "Show current device groups",
	Long:  "Show current device groups.",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		api := api.CtxGetApi(cmd.Context())
		showGroups(api.Configs())
	},
}

func init() {
	ConfigsCmd.AddCommand(listGroups)
}

func showGroups(capi api.ConfigsApi) {
	groups, err := capi.ListGroups()
	cobra.CheckErr(err)
	for _, name := range groups {
		fmt.Println(name)
	}
}
