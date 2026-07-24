// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package updates

import (
	"fmt"
	"strings"

	"github.com/foundriesio/update-server/cli/api"
	"github.com/spf13/cobra"
)

var showRolloutCmd = &cobra.Command{
	Use:   "show-rollout <tag> <update-name> <rollout>",
	Short: "Show details for a specific rollout",
	Long:  `Display detailed information about a rollout including UUIDs, groups, and effective devices`,
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		api := api.CtxGetApi(cmd.Context())
		showRollout(api.Updates(), args[0], args[1], args[2])
		return nil
	},
}

func init() {
	showRolloutCmd.Flags().BoolVar(&showReportSamplings, "show-samplings", false, "Print the sampling of devices in the rollout summary")
	UpdatesCmd.AddCommand(showRolloutCmd)
}

func showRollout(updates api.UpdatesApi, tag, updateName, rollout string) {
	rolloutData, err := updates.GetRollout(tag, updateName, rollout)
	cobra.CheckErr(err)

	fmt.Printf("Rollout: %s\n", rollout)
	fmt.Printf("Update: %s\n", updateName)
	fmt.Printf("Tag: %s\n", tag)
	fmt.Printf("Committed: %v\n\n", rolloutData.Commit)

	report, err := updates.GetRolloutReport(tag, updateName, rollout)
	if err != nil {
		fmt.Printf("Error fetching rollout report: %v\n", err)
		return
	}
	fmt.Println("Summary:")
	for status, summary := range report.Summaries {
		fmt.Printf("  %s: %d\n", status, summary.TotalDevices)
		if showReportSamplings {
			fmt.Printf("    Sampling of devices: %s\n", strings.Join(summary.Sampling, ", "))
		}
	}
	if len(report.MissingDevices) > 0 {
		fmt.Printf("\n  Missing Devices: %d\n", len(report.MissingDevices))
		if showReportSamplings {
			fmt.Printf("    %s\n", strings.Join(report.MissingDevices, ", "))
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
