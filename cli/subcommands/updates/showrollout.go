// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package updates

import (
	"fmt"

	"github.com/foundriesio/update-server/cli/api"
	"github.com/spf13/cobra"
)

var showRolloutCmd = &cobra.Command{
	Use:   "show-rollout <update-name> <rollout>",
	Short: "Show details for a specific rollout",
	Long:  `Display detailed information about a rollout including UUIDs, groups, and effective devices`,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		api := api.CtxGetApi(cmd.Context())
		showRollout(api.Updates(), args[0], args[1])
		return nil
	},
}

func init() {
	showRolloutCmd.Flags().BoolVar(&showDevices, "show-devices", false, "Print the device UUIDs in the rollout summary")
	UpdatesCmd.AddCommand(showRolloutCmd)
}

func showRollout(updates api.UpdatesApi, updateName, rollout string) {
	rolloutData, err := updates.GetRollout(updateName, rollout)
	cobra.CheckErr(err)

	fmt.Printf("Committed: %v\n\n", rolloutData.Commit)

	summary, err := updates.GetRolloutSummary(updateName, rollout)
	if err != nil {
		fmt.Printf("Error fetching rollout report: %v\n", err)
		return
	}
	fmt.Println("Summary:")
	for status, count := range summary.Status {
		fmt.Printf("  %s: %d\n", status, count)
		if showDevices {
			fmt.Printf("    looking up devices ...")
			devices, err := updates.GetDevicesForRolloutStatus(updateName, rollout, status)
			if err != nil {
				fmt.Printf("\rError fetching devices for status %s: %v\n", status, err)
			} else {
				fmt.Printf("\r")
				for _, device := range devices {
					fmt.Printf("    %s\n", device)
				}
			}
		}
	}
	fmt.Println()

	if len(rolloutData.Groups) > 0 {
		fmt.Println("Groups:")
		for _, group := range rolloutData.Groups {
			fmt.Printf("  - %s\n", group)
		}
		fmt.Println()
	}

	if len(rolloutData.Uuids) > 0 {
		fmt.Println("Device UUIDs:")
		for _, uuid := range rolloutData.Uuids {
			fmt.Printf("  - %s\n", uuid)
		}
		fmt.Println()
	}

	if len(rolloutData.Effect) > 0 {
		fmt.Printf("Rolled out to %d devices:\n", len(rolloutData.Effect))
		for _, uuid := range rolloutData.Effect {
			fmt.Printf("  - %s\n", uuid)
		}
	} else {
		fmt.Println("The rollout is request is still being processed.")
	}
}
