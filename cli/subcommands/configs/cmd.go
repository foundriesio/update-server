// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package configs

import (
	"github.com/spf13/cobra"

	"github.com/foundriesio/dg-satellite/cli/api"
)

var ConfigsCmd = &cobra.Command{
	Use:    "configs",
	Short:  "Manage configs",
	Long:   `Commands for managing configs in the Satellite server`,
	Hidden: true,
}

func addSpecificFlags(cmd *cobra.Command) {
	cmd.Flags().StringP("group", "g", "", "Use a group config for a given group name.")
	cmd.Flags().StringP("device", "d", "", "Use a device config for a given device UUID.")
	cmd.MarkFlagsMutuallyExclusive("group", "device")
}

func getSpecificApi(cmd *cobra.Command) api.SpecificConfigsApi {
	api := api.CtxGetApi(cmd.Context()).Configs()
	if uuid, _ := cmd.Flags().GetString("device"); len(uuid) > 0 {
		return api.Device(uuid)
	} else if name, _ := cmd.Flags().GetString("group"); len(name) > 0 {
		return api.Group(name)
	}
	return api.Factory()
}
