// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package login

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/foundriesio/update-server/cli/config"
)

var LoginCmd = &cobra.Command{
	Use:   "login <context-name> <server-url>",
	Short: "Configure authentication for a server",
	Long: `Login to a Foundries Update Server by configuring a context with authentication.

This command will guide you through the authentication process and save
the configuration to ~/.config/satcli.yaml.`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		contextName := args[0]
		serverURL := args[1]

		token, _ := cmd.Flags().GetString("token")
		setDefault, _ := cmd.Flags().GetBool("set-default")
		configPath, _ := cmd.Flags().GetString("config")

		cobra.CheckErr(login(contextName, serverURL, token, configPath, setDefault))
	},
}

func init() {
	LoginCmd.Flags().String("token", "", "API token for authentication (required for now)")
	LoginCmd.Flags().Bool("set-default", true, "Set this context as the default")
	LoginCmd.Flags().String("config", "", "Specify the configuration file to use")
	cobra.CheckErr(LoginCmd.MarkFlagRequired("token"))
}

func login(contextName, serverURL, token, configPath string, setDefault bool) error {
	if token == "" {
		return fmt.Errorf("--token is required")
	}

	// Load existing config or create new one
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			cfg = &config.Config{
				Contexts: make(map[string]config.Context),
			}
		} else {
			return fmt.Errorf("failed to load config: %w", err)
		}
	}

	if cfg.Contexts == nil {
		cfg.Contexts = make(map[string]config.Context)
	}
	cfg.Contexts[contextName] = config.Context{
		URL:   serverURL,
		Token: token,
	}

	if setDefault {
		cfg.ActiveContext = contextName
	}

	if err := config.SaveConfig(configPath, cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Successfully configured context '%s'\n", contextName)
	fmt.Printf("  Server URL: %s\n", serverURL)
	if setDefault {
		fmt.Printf("  Set as default context\n")
	}

	return nil
}
