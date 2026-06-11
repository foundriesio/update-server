// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package devices

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/foundriesio/update-server/cli/api"
	"github.com/spf13/cobra"
)

var showCmd = &cobra.Command{
	Use:   "show <uuid>",
	Short: "Show details for a specific device",
	Long:  `Display detailed information about a device`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		api := api.CtxGetApi(cmd.Context())
		aktoml, _ := cmd.Flags().GetBool("aktoml")
		hwinfo, _ := cmd.Flags().GetBool("hwinfo")
		showDevice(api.Devices(), args[0], aktoml, hwinfo)
	},
}

func init() {
	DevicesCmd.AddCommand(showCmd)
	showCmd.Flags().Bool("hwinfo", false, "Display hardware information")
	showCmd.Flags().Bool("aktoml", false, "Display devices' reported configuration")
}

func showDevice(devices api.DeviceApi, uuid string, aktoml, hwinfo bool) {
	device, err := devices.Get(uuid)
	cobra.CheckErr(err)

	fmt.Printf("UUID:         %s\n", device.Uuid)
	fmt.Printf("Target:       %s\n", device.Target)
	fmt.Printf("Tag:          %s\n", device.Tag)
	fmt.Printf("Is Prod:      %v\n", device.IsProd)

	if device.Status != nil {
		status := device.Status.Status + "; " + device.Status.DeviceTime
		if len(device.Status.TargetName) > 0 {
			status = status + "; " + device.Status.TargetName
		}
		fmt.Printf("Status:       %s\n", status)
	}
	if device.CreatedAt > 0 {
		fmt.Printf("Created At:   %s\n", time.Unix(device.CreatedAt, 0).Format("2006-01-02 15:04:05"))
	}
	if device.LastSeen > 0 {
		fmt.Printf("Last Seen:    %s\n", time.Unix(device.LastSeen, 0).Format("2006-01-02 15:04:05"))
	}

	if device.UpdateName != "" {
		fmt.Printf("Update Name:  %s\n", device.UpdateName)
	}
	if device.OstreeHash != "" {
		fmt.Printf("OSTree Hash:  %s\n", device.OstreeHash)
	}

	if len(device.Apps) > 0 {
		fmt.Printf("Apps:         %s\n", strings.Join(device.Apps, ", "))
	}

	if len(device.Labels) > 0 {
		fmt.Println("\nLabels:")
		for k, v := range device.Labels {
			fmt.Printf("  %s: %s\n", k, v)
		}
	}

	if device.NetInfo != "" {
		fmt.Println("\nNetwork Info:")
		fmt.Println(" ", device.NetInfo)
	}

	if aktoml && device.Aktoml != "" {
		fmt.Println("\nAktoml:")
		lines := strings.SplitSeq(device.Aktoml, "\n")
		for line := range lines {
			fmt.Printf(" | %s\n", line)
		}
	}

	if hwinfo && device.HwInfo != "" {
		fmt.Println("\nHardware Info:")
		var hwinfo map[string]any
		if err := json.Unmarshal([]byte(device.HwInfo), &hwinfo); err != nil {
			fmt.Println("  (invalid hwinfo data):")
			fmt.Println("  ", device.HwInfo)
		} else {
			hwinfoBytes, err := json.MarshalIndent(hwinfo, "  ", "  ")
			cobra.CheckErr(err)
			fmt.Println(" ", string(hwinfoBytes))
		}
	}
}
