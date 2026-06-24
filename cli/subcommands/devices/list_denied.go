// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package devices

import (
	"fmt"

	"github.com/foundriesio/update-server/cli/api"
	"github.com/spf13/cobra"
)

var listDeniedCmd = &cobra.Command{
	Use:   "list-denied",
	Short: "List denied devices",
	Long: `List devices on the denied list. A denied device is prevented from
accessing the backend via mTLS. Use 'devices delete-denied' to remove a
device from the list and allow it back.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		a := api.CtxGetApi(cmd.Context())
		listDeniedDevices(a.Devices())
		return nil
	},
}

func init() {
	DevicesCmd.AddCommand(listDeniedCmd)
}

func listDeniedDevices(dapi api.DeviceApi) {
	uuids, err := dapi.ListDenied()
	cobra.CheckErr(err)
	for _, uuid := range uuids {
		fmt.Println(uuid)
	}
}
