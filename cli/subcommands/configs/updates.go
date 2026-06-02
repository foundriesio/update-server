// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package configs

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	toml "github.com/pelletier/go-toml"
	"github.com/spf13/cobra"

	"github.com/foundriesio/dg-satellite/cli/api"
)

var updatesCmd = &cobra.Command{
	Use:   "updates [ -g <group-name> | -d <device-uuid> ]",
	Short: "Configure a special sota.toml config file used for updates on a global, group, or device level",
	Long: `Configure a special sota.toml config file used for updates on a global, group, or device 

	By default, the command sets global configs.
	If the --group or --device argument is provided, it sets group or device configs.
	If a specified group does not exist - a command creates it.`,
	Example: `
	# Make devices start taking updates from Targets tagged with "devel":
	satcli configs updates --group beta --tag devel

	# Set the Compose apps that devices will run:
	satcli configs updates --group beta --apps shellhttpd

	# Set the Compose apps and the tag for devices:
	satcli configs updates --group beta --apps shellhttpd --tag master

	# There are two special characters: "," and "-".
	# Providing a "," sets the Compose apps to "none" for devices.
	# This will make the device run no apps:
	satcli configs updates --apps ,

	# Providing a "-" sets the Compose apps to "preset-apps" (all apps on most devices).
	satcli configs updates --apps -

	# Set the device tag to a "preset-tag",
	satcli configs updates --tag -

	# The system looks in the following locations to get the complete config:
	# - /usr/lib/sota/conf.d/
	# - /var/sota/sota.toml
	# - /etc/sota/conf.d/`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		api := getSpecificApi(cmd)
		reason, _ := cmd.Flags().GetString("reason")
		apps, tag, err := validateUpdatesArgs(cmd)
		if err != nil {
			return err
		}
		setUpdates(api, apps, tag, reason)
		return nil
	},
}

func init() {
	ConfigsCmd.AddCommand(updatesCmd)
	addSpecificFlags(updatesCmd)
	updatesCmd.Flags().StringP("apps", "", "", "A comma-separated list of apps to run on the device.")
	updatesCmd.Flags().StringP("tag", "", "", "A tag to follow on the device.")
	updatesCmd.Flags().StringP("reason", "m",
		"Override aktualizr-lite update configuration",
		"Add a message to store as the \"reason\" for this change")
}

func setUpdates(capi api.SpecificConfigsApi, apps, tag, reason string) {
	cfg, err := capi.Get()
	cobra.CheckErr(err)

	if cfg.Files == nil {
		cfg.Files = make(map[string]api.ConfigFile)
	}

	const (
		sotaOverride          = "z-50-fioctl.toml"
		sotaOverrideOnChanged = "/usr/share/fioconfig/handlers/aktualizr-toml-update"
	)
	var sotaFile api.ConfigFile
	for name, val := range cfg.Files {
		if name == sotaOverride {
			sotaFile = val
			break
		}
	}
	sota, err := toml.Load(sotaFile.Value)
	cobra.CheckErr(err)

	switch apps {
	case "": // noop
	case "-":
		if sota.Has("pacman.compose_apps") {
			cobra.CheckErr(sota.Delete("pacman.compose_apps"))
		}
	case ",":
		apps = ""
		fallthrough
	default:
		sota.Set("pacman.compose_apps", apps)
	}

	switch tag {
	case "": // noop
	case "-":
		if sota.Has("pacman.tags") {
			cobra.CheckErr(sota.Delete("pacman.tags"))
		}
	default:
		sota.Set("pacman.tags", tag)
	}

	newSotaContent, err := sota.ToTomlString()
	cobra.CheckErr(err)
	if newSotaContent == sotaFile.Value {
		fmt.Println("No changes found.")
		return
	}
	cfg.Files[sotaOverride] = api.ConfigFile{
		Value:       newSotaContent,
		Unencrypted: true,
		OnChanged:   []string{sotaOverrideOnChanged},
	}
	cobra.CheckErr(capi.Put(api.ConfigFileSet{Files: cfg.Files, Reason: reason}))
}

var (
	reAppsPattern = regexp.MustCompile(`^[a-zA-Z0-9-_,]+$`)
	reTagPattern  = regexp.MustCompile(`^[a-zA-Z0-9_\-\.\+]+$`)
)

func validateUpdatesArgs(cmd *cobra.Command) (apps, tag string, err error) {
	apps, _ = cmd.Flags().GetString("apps")
	tag, _ = cmd.Flags().GetString("tag")
	apps = strings.TrimSpace(apps)
	tag = strings.TrimSpace(tag)
	switch {
	case len(apps) == 0 && len("tag") == 0:
		err = errors.New("either apps or tag must be specified")
	case len(apps) > 0 && !reAppsPattern.MatchString(apps):
		err = fmt.Errorf("invalid value for apps: %s; must be %s", apps, reAppsPattern.String())
	case len(tag) > 0 && !reTagPattern.MatchString(tag):
		err = fmt.Errorf("invalid value for tag: %s; must be %s", tag, reTagPattern.String())
	}
	return
}
