// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package devices

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/foundriesio/update-server/cli/api"
	"github.com/foundriesio/update-server/cli/subcommands"
	"github.com/spf13/cobra"
)

var testsCmd = &cobra.Command{
	Use:   "tests <uuid> [test-id] [artifact]",
	Short: "Show device tests",
	Long:  `List all tests uploaded by a device, show details for a specific test, or show the contents of a test artifact`,
	Args:  cobra.RangeArgs(1, 3),
	Run: func(cmd *cobra.Command, args []string) {
		api := api.CtxGetApi(cmd.Context())
		switch len(args) {
		case 1:
			listTests(api.Devices(), args[0])
		case 2:
			showTest(api.Devices(), args[0], args[1])
		default:
			showTestArtifact(api.Devices(), args[0], args[1], args[2])
		}
	},
}

func init() {
	DevicesCmd.AddCommand(testsCmd)
}

func listTests(devices api.DeviceApi, uuid string) {
	tests, err := devices.Tests(uuid)
	cobra.CheckErr(err)

	if len(tests) == 0 {
		fmt.Println("No tests found for this device")
		return
	}

	t := subcommands.NewTableWriter([]string{"NAME", "STATUS", "ID", "CREATED"})

	for _, test := range tests {
		created := time.Unix(test.CreatedOn, 0).Format("2006-01-02 15:04:05")
		t.AddRow(test.Name, test.Status, test.Uuid, created)
	}
	t.Render()
}

func showTest(devices api.DeviceApi, uuid, testId string) {
	test, err := devices.Test(uuid, testId)
	cobra.CheckErr(err)

	fmt.Printf("Name:      %s\n", test.Name)
	fmt.Printf("Status:    %s\n", test.Status)
	fmt.Printf("Id:        %s\n", test.Uuid)
	fmt.Printf("Created:   %s\n", time.Unix(test.CreatedOn, 0).Format("2006-01-02 15:04:05"))
	if test.CompletedOn != nil {
		fmt.Printf("Completed: %s\n", time.Unix(*test.CompletedOn, 0).Format("2006-01-02 15:04:05"))
	}

	if test.Details != "" {
		fmt.Println("\nDetails:")
		for line := range strings.SplitSeq(test.Details, "\n") {
			fmt.Printf("  | %s\n", line)
		}
	}

	if len(test.Results) > 0 {
		fmt.Println("\nResults:")
		rt := subcommands.NewTableWriter([]string{"NAME", "STATUS"})
		for _, r := range test.Results {
			rt.AddRow(r.Name, r.Status)
		}
		rt.Render()
	}

	if len(test.Artifacts) > 0 {
		fmt.Println("\nArtifacts:")
		for _, a := range test.Artifacts {
			fmt.Printf("\t%s\n", a)
		}
	}
}

func showTestArtifact(devices api.DeviceApi, uuid, testId, artifact string) {
	body, err := devices.TestArtifact(uuid, testId, artifact)
	cobra.CheckErr(err)
	defer func() {
		if err := body.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to close response body: %v\n", err)
		}
	}()

	_, err = io.Copy(os.Stdout, body)
	cobra.CheckErr(err)
}
