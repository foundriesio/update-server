// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package updates

import (
	"fmt"
	"strings"

	"github.com/foundriesio/update-server/cli/api"
	"github.com/spf13/cobra"
)

var createRolloutCmd = &cobra.Command{
	Use:   "create-rollout <ci|prod> <tag> <update-name> <rollout-name>",
	Short: "Create a new rollout for an update",
	Long:  `Create a new rollout specifying device UUIDs and/or groups to target`,
	Args:  cobra.ExactArgs(4),
	RunE: func(cmd *cobra.Command, args []string) error {
		api := api.CtxGetApi(cmd.Context())
		prodType := args[0]

		// Validate prod type
		if prodType != "ci" && prodType != "prod" {
			return fmt.Errorf("first argument must be 'ci' or 'prod', got '%s'", prodType)
		}

		uuids, _ := cmd.Flags().GetString("uuids")
		groups, _ := cmd.Flags().GetString("groups")

		updates := api.Updates(prodType)
		cobra.CheckErr(createRollout(updates, args[1], args[2], args[3], uuids, groups))
		return nil
	},
}

func init() {
	UpdatesCmd.AddCommand(createRolloutCmd)
	createRolloutCmd.Flags().String("uuids", "", "Comma-separated list of device UUIDs")
	createRolloutCmd.Flags().String("groups", "", "Comma-separated list of device groups")
}

func createRollout(updates api.UpdatesApi, tag, updateName, rolloutName, uuidsStr, groupsStr string) error {
	if uuidsStr == "" && groupsStr == "" {
		return fmt.Errorf("at least one of --uuids or --groups must be specified")
	}

	var uuids []string
	if uuidsStr != "" {
		for uuid := range strings.SplitSeq(uuidsStr, ",") {
			trimmed := strings.TrimSpace(uuid)
			if trimmed != "" {
				uuids = append(uuids, trimmed)
			}
		}
	}

	var groups []string
	if groupsStr != "" {
		for group := range strings.SplitSeq(groupsStr, ",") {
			trimmed := strings.TrimSpace(group)
			if trimmed != "" {
				groups = append(groups, trimmed)
			}
		}
	}

	rollout := api.Rollout{
		Uuids:  uuids,
		Groups: groups,
	}

	cobra.CheckErr(updates.CreateRollout(tag, updateName, rolloutName, rollout))
	return nil
}
