// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package configs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/foundriesio/update-server/cli/api"
)

var setCmd = &cobra.Command{
	Use:   "set [ -g <group-name> | -d <device-uuid> ] <file1=content1> [ <file2=content2> ... ]",
	Short: "Create a global, group, or device configs",
	Long: `Create a global, group, or device configs.

	By default, the command sets global configs.
	If the --group or --device argument is provided, it sets group or device configs.
	If a specified group does not exist, it is created.

	By default, provided config is merged into the existing config:
	- new files are added,
	- files with the same name are replaces,
	- existing files without a match are preserved.
	A user can change this behavior to an overwrite by --replace option.`,
	Example: `
	# Basic use
	satcli configs set npmtok="root" readme.md==./readme.md

	There are several ways to pass a file's content into this command:
	- with the filename="filecontent" format, content is passed directly.
	- with the filename==/path/to/file format, content is read from the specified file path.

	# The configuration format allows specifying what command to run
	# after a configuration file is updated on the device.
	# To take advantage of this, the "--raw" flag must be used.
	cat >tmp.json <<EOF
	{
	"reason": "I want to use the on-changed attribute",
	"files": [
	{
	"name": "npmtok",
	"value": "root",
	"on-changed": ["/usr/share/fioconfig/handlers/custom-handler", "/tmp/npmtok-changed"]
	}
	]
	}
	> EOF
	satcli configs set --raw ./tmp.json

	# satcli will read in tmp.json and upload it to the OTA server.
	# Instead of using ./tmp.json, the command can take a "-" and will read the content from STDIN instead of a file.`,

	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		api := getSpecificApi(cmd)
		raw, _ := cmd.Flags().GetBool("raw")
		replace, _ := cmd.Flags().GetBool("replace")
		reason, _ := cmd.Flags().GetString("reason")
		return setConfigs(api, args, raw, replace, reason)
	},
}

func init() {
	ConfigsCmd.AddCommand(setCmd)
	addSpecificFlags(setCmd)
	setCmd.Flags().BoolP("raw", "", false, "Use raw configuration file.")
	setCmd.Flags().BoolP("replace", "", false, "Replace existing config rather than merge it.")
	setCmd.Flags().StringP("reason", "m", "", "Add a message to store as the \"reason\" for this change")
}

func setConfigs(capi api.SpecificConfigsApi, files []string, raw, replace bool, reason string) error {
	var (
		cfg  api.ConfigFileSet
		data []byte
		err  error
	)
	cfg.Reason = reason
	if raw {
		if raw && len(files) != 1 {
			return errors.New("raw file only accepts one file argument")
		}
		path := files[0]
		if path == "-" {
			data, err = io.ReadAll(os.Stdin)
		} else {
			data, err = os.ReadFile(path)
		}
		cobra.CheckErr(err)
		cobra.CheckErr(json.Unmarshal(data, &cfg.Files))
	} else {
		cfg.Files = make(map[string]api.ConfigFile, len(files))
		for _, keyval := range files {
			parts := strings.SplitN(keyval, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid file=content argument: %s", keyval)
			}
			name := parts[0]
			// support for filename=filecontent format
			content := parts[1]
			if len(content) > 0 && content[0] == '=' {
				// support for filename==/file/path.ext format
				data, err = os.ReadFile(content[1:])
				cobra.CheckErr(err)
				content = string(data)
			}
			cfg.Files[name] = api.ConfigFile{Value: content, Unencrypted: true}
		}
	}
	if !replace {
		// This might be less efficient than a server-side patch for large config files.
		// But it is a good start which requires the least effort.
		existing, err := capi.Get()
		cobra.CheckErr(err)
		for name, val := range existing.Files {
			if _, ok := cfg.Files[name]; !ok {
				cfg.Files[name] = val
			}
		}
	}
	cobra.CheckErr(capi.Put(cfg))
	return nil
}
