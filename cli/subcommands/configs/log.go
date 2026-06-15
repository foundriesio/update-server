// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package configs

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/foundriesio/update-server/cli/api"
)

var logCmd = &cobra.Command{
	Use:   "log [ -g <group-name> | -d <device-uuid> ]",
	Short: "Show a global, group, or device configs history",
	Long: `Show a global, group, or device configs history.

	By default, the command shows global configs history without files contents.
	If the --group or --device argument is provided, it shows group or device configs history.
	If --show-files argument is provided, it shows configs files contents for each history item.
	Use the --limit argument to control how many history items are being shown.

	History items are separated by a "----" line, with blank lines above and below it.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		api := getSpecificApi(cmd)
		limit, _ := cmd.Flags().GetInt("limit")
		showFiles, _ := cmd.Flags().GetBool("show-files")
		showConfigsHistory(api, limit, showFiles)
	},
}

func init() {
	ConfigsCmd.AddCommand(logCmd)
	addSpecificFlags(logCmd)
	logCmd.Flags().IntP("limit", "n", 0, "Limit how many history items are shown.")
	logCmd.Flags().BoolP("show-files", "s", false, "Show config files contents for each history item.")
}

func showConfigsHistory(capi api.SpecificConfigsApi, limit int, showFiles bool) {
	history, err := capi.GetHistory(limit, showFiles)
	cobra.CheckErr(err)
	if len(history) == 0 {
		fmt.Println("No configuration has been created yet.")
		return
	}
	for idx, cfg := range history {
		if idx > 0 {
			fmt.Print("\n----\n\n") // A linter would complain about a trailing newline in Println.
		}
		printConfigsInfo(cfg)
		if showFiles {
			printConfigs(cfg.Files)
		}
	}
}
