// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package updates

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/foundriesio/update-server/cli/api"
	"github.com/spf13/cobra"
)

var showTufDetails bool

var showCmd = &cobra.Command{
	Use:   "show <tag> <update-name>",
	Short: "Show details for an update",
	Long:  `Display details about an update from its TUF metadata`,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		api := api.CtxGetApi(cmd.Context())
		showUpdate(api.Updates(), args[0], args[1])
		return nil
	},
}

func init() {
	showCmd.Flags().BoolVar(&showTufDetails, "tuf-details", false, "Print the raw TUF metadata")
	UpdatesCmd.AddCommand(showCmd)
}

func showUpdate(updates api.UpdatesApi, tag, updateName string) {
	tuf, err := updates.GetTuf(tag, updateName)
	cobra.CheckErr(err)

	fmt.Println("# Details")
	fmt.Printf("Update: %s\n", updateName)
	fmt.Printf("Tag: %s\n\n", tag)
	showRollouts(updates, tag, updateName)

	fmt.Println("# TUF metadata")
	if showTufDetails {
		raw, err := json.MarshalIndent(tuf, "", "  ")
		cobra.CheckErr(err)
		fmt.Println(string(raw))
		return
	}

	name, target := firstTarget(tuf)
	if target == nil {
		fmt.Println("No targets found in the TUF metadata")
		return
	}
	custom, _ := target["custom"].(map[string]any)

	fmt.Printf("  Latest target name: %s\n", name)
	fmt.Printf("  Version: %s\n", customString(custom, "version"))
	fmt.Println()

	showExpires(tuf)

	printList("Hardware IDs:", custom, "hardwareIds")
	printList("Tags:", custom, "tags")

	fmt.Printf("  OSTree hash: %s\n", targetSha256(target))

	fmt.Println("  Apps:")
	printApps(custom)
}

func showRollouts(updates api.UpdatesApi, tag, updateName string) {
	rollouts, err := updates.Get(tag, updateName)
	cobra.CheckErr(err)

	if len(rollouts) == 0 {
		fmt.Print("# Rollouts: None\n\n")
		return
	}

	fmt.Println("# Rollout history")
	for _, rollout := range rollouts {
		fmt.Printf("  %s\n", rollout)
	}
	fmt.Println()
}

func showExpires(tuf api.UpdateTuf) {
	fmt.Println("  Expirations:")
	fmt.Printf("    Root:      %s\n", tufExpires(tuf, "root.json"))
	fmt.Printf("    Timestamp: %s\n", tufExpires(tuf, "timestamp.json"))
	fmt.Printf("    Snapshot:  %s\n", tufExpires(tuf, "snapshot.json"))
	fmt.Printf("    Targets:   %s\n\n", tufExpires(tuf, "targets.json"))
}

// tufExpires returns the "signed.expires" value for the given TUF role file.
func tufExpires(tuf api.UpdateTuf, file string) string {
	role, ok := tuf[file]
	if !ok {
		return "unknown"
	}
	signed, ok := role["signed"].(map[string]any)
	if !ok {
		return "unknown"
	}
	if expires, ok := signed["expires"].(string); ok {
		return expires
	}
	return "unknown"
}

// firstTarget returns the name and metadata of the first target found in the
// targets.json metadata.
func firstTarget(tuf api.UpdateTuf) (string, map[string]any) {
	targetsJson, ok := tuf["targets.json"]
	if !ok {
		return "", nil
	}
	signed, ok := targetsJson["signed"].(map[string]any)
	if !ok {
		return "", nil
	}
	targets, ok := signed["targets"].(map[string]any)
	if !ok {
		return "", nil
	}
	for name, target := range targets {
		if t, ok := target.(map[string]any); ok {
			return name, t
		}
	}
	return "", nil
}

func customString(custom map[string]any, field string) string {
	if custom == nil {
		return ""
	}
	if s, ok := custom[field].(string); ok {
		return s
	}
	return ""
}

func printList(header string, custom map[string]any, field string) {
	fmt.Printf("  %s\n", header)
	if custom == nil {
		return
	}
	values, ok := custom[field].([]any)
	if !ok {
		return
	}
	for _, value := range values {
		if s, ok := value.(string); ok && s != "" {
			fmt.Printf("    %s\n", s)
		}
	}
	fmt.Println()
}

func targetSha256(target map[string]any) string {
	hashes, ok := target["hashes"].(map[string]any)
	if !ok {
		return ""
	}
	if h, ok := hashes["sha256"].(string); ok {
		return h
	}
	return ""
}

func printApps(custom map[string]any) {
	apps := make(map[string]string)
	if custom != nil {
		if dockerApps, ok := custom["docker_compose_apps"].(map[string]any); ok {
			for name, val := range dockerApps {
				if appMap, ok := val.(map[string]any); ok {
					if uri, ok := appMap["uri"].(string); ok {
						apps[name] = uri
					}
				}
			}
		}
	}
	if len(apps) == 0 {
		fmt.Println("    None")
		return
	}
	names := make([]string, 0, len(apps))
	for name := range apps {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Printf("    %s: %s\n", name, apps[name])
	}
}
