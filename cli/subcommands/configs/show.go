// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package configs

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/foundriesio/update-server/cli/api"
)

var showCmd = &cobra.Command{
	Use:   "show [ -g <group-name> | -d <device-uuid> ]",
	Short: "Show a global, group, or device configs",
	Long: `Show a global, group, or device configs.

	By default, the command shows global configs.
	If the --group or --device argument is provided, it shows group or device configs.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		api := getSpecificApi(cmd)
		showConfigs(api)
	},
}

func init() {
	ConfigsCmd.AddCommand(showCmd)
	addSpecificFlags(showCmd)
}

func showConfigs(capi api.SpecificConfigsApi) {
	cfg, err := capi.Get()
	cobra.CheckErr(err)
	if cfg.CreatedAt == 0 && len(cfg.Files) == 0 {
		// Files can be empty when configs were deleted, but CreatedAt is always set if there were any changes.
		fmt.Println("No configuration has been created yet.")
		return
	}
	printConfigsInfo(cfg)
	printConfigs(cfg.Files)
}

func printConfigsInfo(cfg api.ConfigFileSet) {
	fmt.Printf("Reason: %s\n", cfg.Reason)
	fmt.Printf("Created At: %s\n", formatTimestamp(cfg.CreatedAt))
	fmt.Printf("Created By: %s\n", cfg.CreatedBy)
}

func printConfigs(cfg map[string]api.ConfigFile) {
	fmt.Println("Files:")
	for name, file := range cfg {
		if len(file.OnChanged) == 0 {
			fmt.Printf("\t%s\n", name)
		} else {
			fmt.Printf("\t%s - %v\n", name, file.OnChanged)
		}
		if file.Unencrypted {
			for _, line := range strings.Split(file.Value, "\n") {
				fmt.Printf("\t | %s\n", line)
			}
		}
	}
}

func formatTimestamp(timestamp int64) string {
	return time.Unix(timestamp, 0).UTC().Format(time.RFC3339)
}
