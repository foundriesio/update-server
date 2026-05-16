// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package configs

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/foundriesio/dg-satellite/cli/api"
)

var appliedCmd = &cobra.Command{
	Use:   "applied <device-uuid>",
	Short: "Show the applied configuration for a device",
	Long: `Show the merged (factory + group + device) configuration that was most
recently delivered to the device, along with the time it was applied.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		a := api.CtxGetApi(cmd.Context()).Configs().Device(args[0])
		getDeviceApplied(a)
	},
}

func init() {
	ConfigsCmd.AddCommand(appliedCmd)
}

func getDeviceApplied(a api.DeviceConfigsApi) {
	applied, err := a.GetApplied()
	cobra.CheckErr(err)
	if applied == nil {
		fmt.Println("No configuration has been applied to this device yet.")
		return
	}
	appliedAt := time.Unix(applied.AppliedAt, 0)
	fmt.Printf("Applied at: %s\n", appliedAt.Format(time.RFC1123))
	cfg := make(api.ConfigFileMap, len(applied.Files))
	for k, v := range applied.Files {
		cfg[k] = *v
	}
	printConfigs(cfg)
}
