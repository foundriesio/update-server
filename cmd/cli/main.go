// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"fmt"
	"os"

	"github.com/foundriesio/update-server/cli/api"
	"github.com/foundriesio/update-server/cli/config"
	"github.com/foundriesio/update-server/cli/subcommands/configs"
	"github.com/foundriesio/update-server/cli/subcommands/devices"
	"github.com/foundriesio/update-server/cli/subcommands/login"
	"github.com/foundriesio/update-server/cli/subcommands/updates"
	"github.com/foundriesio/update-server/version"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "satcli",
	Short: "A command line interface to the Satellite Server",
	Long: `satcli is a command-line interface for managing devices, updates,
and other resources on a Satellite server.

Configuration is stored in $HOME/.config/satcli.yaml`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip config logic for login and version commands
		if cmd.Name() == "login" || cmd.Name() == "version" {
			return nil
		}

		configPath, err := cmd.Flags().GetString("config")
		if err != nil {
			return fmt.Errorf("failed to get config flag: %w", err)
		}
		cfg, err := config.LoadConfig(configPath)
		if err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		contextName, err := cmd.Flags().GetString("context")
		if err != nil {
			return fmt.Errorf("failed to get context flag: %w", err)
		}

		appctx, err := cfg.GetContext(contextName)
		if err != nil {
			return fmt.Errorf("failed to get current context: %w", err)
		}

		client := api.NewClient(*appctx)

		ctx := api.CtxWithApi(cmd.Context(), client)
		cmd.SetContext(ctx)

		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringP("context", "c", "", "Specify the context to use from the configuration file")
	rootCmd.PersistentFlags().StringP("config", "f", "", "Specify the configuration file to use")

	rootCmd.AddCommand(login.LoginCmd)
	rootCmd.AddCommand(configs.ConfigsCmd)
	rootCmd.AddCommand(devices.DevicesCmd)
	rootCmd.AddCommand(updates.UpdatesCmd)
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the version of satcli",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version.Version)
		},
	})
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
