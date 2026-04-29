// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package configs

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/foundriesio/dg-satellite/cli/api"
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
	printConfigs(cfg)
}

func printConfigs(configs api.ConfigFileSet) {
	fmt.Println("Files:")
	for name, file := range configs {
		if len(file.OnChanged) == 0 {
			fmt.Printf("\t%s\n", name)
		} else {
			fmt.Printf("\t%s - %v\n", name, file.OnChanged)
		}
		if file.Unencrypted != nil && *file.Unencrypted {
			for _, line := range strings.Split(file.Value, "\n") {
				fmt.Printf("\t | %s\n", line)
			}
		}
	}
}
