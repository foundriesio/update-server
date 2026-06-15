// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package updates

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/foundriesio/update-server/cli/api"
	"github.com/foundriesio/update-server/cli/subcommands"
)

var createCmd = &cobra.Command{
	Use:   "upload <ci|prod> <tag> <update-name> <directory>",
	Short: "Upload an offline update",
	Long:  `Create an update on Update server by uploading the offline update found in the directory.`,
	Args:  cobra.ExactArgs(4),
	RunE: func(cmd *cobra.Command, args []string) error {
		a := api.CtxGetApi(cmd.Context())
		prodType := args[0]
		if prodType != "ci" && prodType != "prod" {
			return fmt.Errorf("first argument must be 'ci' or 'prod', got '%s'", prodType)
		}
		cobra.CheckErr(createUpdate(a.Updates(prodType), args[1], args[2], args[3]))
		return nil
	},
}

func init() {
	UpdatesCmd.AddCommand(createCmd)
}

func createUpdate(updates api.UpdatesApi, tag, updateName, path string) error {
	if stat, err := os.Stat(path); err != nil {
		return fmt.Errorf("failed to stat directory '%s': %w", path, err)
	} else if !stat.Mode().IsDir() {
		return fmt.Errorf("a '%s' is neither a directory nor a symlink to a directory", path)
	}

	progress, sourcer := subcommands.TarProgress(subcommands.ArchiveSourcer(path))
	reader := subcommands.GzipStream(progress.StreamWriter(subcommands.TarStream(sourcer)))
	defer reader.Close() //nolint:errcheck

	stop := make(chan bool)
	done := make(chan bool)
	// See cli/subcommands/configs/upload.go for a comment about how reporter works.
	go progress.Report("Uploaded:", stop, done)

	err := updates.CreateUpdate(tag, updateName, reader)
	stop <- err == nil
	<-done
	return err
}
