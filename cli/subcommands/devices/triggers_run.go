// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package devices

import (
	"crypto/rand"
	"fmt"
	"slices"

	"github.com/foundriesio/update-server/cli/api"
	"github.com/foundriesio/update-server/storage"
	"github.com/spf13/cobra"
)

var runTriggerCmd = &cobra.Command{
	Use:   "run <uuid> <action>",
	Short: "Trigger a remote action on a device",
	Long: `Set a fioconfig entry to run a configured remote action once and capture its output.
Use the returned command ID with 'fiocli devices tests' to check the results.`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		client := api.CtxGetApi(cmd.Context())
		reason, _ := cmd.Flags().GetString("reason")
		runTrigger(client.Configs().Device(args[0]), args[0], args[1], reason)
	},
}

func init() {
	triggersCmd.AddCommand(runTriggerCmd)
	runTriggerCmd.Flags().StringP("reason", "r", "", "The reason for running this action")
}

func runTrigger(configs api.DeviceConfigsApi, uuid, action, reason string) {
	cfg, err := configs.Get()
	cobra.CheckErr(err)

	allowed := configuredTriggers(cfg)
	if !slices.Contains(allowed, action) {
		cobra.CheckErr(fmt.Errorf("invalid action: %s. Allowed actions are: %v", action, allowed))
	}

	const idLen = 15
	const maxActionLen = 48 - idLen - 1
	if len(action) > maxActionLen {
		cobra.CheckErr(fmt.Errorf("action name (%s) too long; maximum length is %d", action, maxActionLen))
	}
	commandId := action + "_" + rand.Text()[:idLen]
	if !storage.TestIdRegex.MatchString(commandId) {
		cobra.CheckErr(fmt.Errorf("action name must contain only ASCII letters, digits, hyphens, and underscores"))
	}

	// Config PUT replaces all files, so preserve the entries read above.
	cfg.Files["fioconfig-oneshot-"+action] = api.ConfigFile{
		Value:       fmt.Sprintf("COMMAND_ID=%s\nCOMMAND_CAPTURE=1\n", commandId),
		Unencrypted: true,
		OnChanged:   []string{"/usr/share/fioconfig/handlers/fioconfig-oneshot", action},
	}
	cobra.CheckErr(configs.Put(api.ConfigFileSet{Files: cfg.Files, Reason: reason}))

	fmt.Println("Config change submitted. Command ID is:", commandId)
	fmt.Printf("Use 'fiocli devices tests %s %s' to check results.\n", uuid, commandId)
}
