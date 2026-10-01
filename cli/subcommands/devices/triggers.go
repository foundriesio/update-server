// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package devices

import (
	"fmt"
	"strings"

	"github.com/foundriesio/update-server/cli/api"
	"github.com/spf13/cobra"
)

var triggersCmd = &cobra.Command{
	Use:   "triggers",
	Short: "Manage device remote actions",
}

var listConfiguredTriggersCmd = &cobra.Command{
	Use:   "list-configured <uuid>",
	Short: "List remote actions configured on a device",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client := api.CtxGetApi(cmd.Context())
		listTriggers(client.Configs().Device(args[0]))
	},
}

func init() {
	DevicesCmd.AddCommand(triggersCmd)
	triggersCmd.AddCommand(listConfiguredTriggersCmd)
}

func listTriggers(configs api.DeviceConfigsApi) {
	cfg, err := configs.Get()
	cobra.CheckErr(err)

	actions := configuredTriggers(cfg)
	if len(actions) == 0 {
		fmt.Println("Remote actions are not configured for this device")
		return
	}

	fmt.Println("Available actions:")
	for _, action := range actions {
		fmt.Println("*", action)
	}
}

func configuredTriggers(cfg api.ConfigFileSet) []string {
	var actions []string
	for action := range strings.SplitSeq(cfg.Files["fio-remote-actions"].Value, ",") {
		if action = strings.TrimSpace(action); action != "" {
			actions = append(actions, action)
		}
	}
	return actions
}
