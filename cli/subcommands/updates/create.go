// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package updates

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/foundriesio/update-server/cli/api"
	"github.com/foundriesio/update-server/cli/subcommands"
)

var createCmd = &cobra.Command{
	Use:   "upload <tag> <update-name> <directory>",
	Short: "Upload an offline update",
	Long:  `Create an update on Update server by uploading the offline update found in the directory.`,
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		opts, err := createOptions(cmd)
		if err != nil {
			return err
		}
		a := api.CtxGetApi(cmd.Context())
		cobra.CheckErr(createUpdate(a.Updates(), args[0], args[1], args[2], opts))
		return nil
	},
}

func init() {
	flags := createCmd.Flags()
	flags.Int("version", 0, "Override the target version (AppVersion)")
	flags.String("name", "", "Override the target name")
	flags.String("hardware-id", "", "Override the hardware id")
	flags.String("ostree-hash", "", "Override the ostree hash")
	flags.StringSlice("apps", nil, "Override docker compose apps as name=sha256 (repeatable or comma separated)")
	UpdatesCmd.AddCommand(createCmd)
}

func createOptions(cmd *cobra.Command) (api.CreateUpdateOptions, error) {
	flags := cmd.Flags()
	var opts api.CreateUpdateOptions
	var err error
	if opts.Version, err = flags.GetInt("version"); err != nil {
		return opts, err
	}
	if opts.Name, err = flags.GetString("name"); err != nil {
		return opts, err
	}
	if opts.HardwareId, err = flags.GetString("hardware-id"); err != nil {
		return opts, err
	}
	if opts.OstreeHash, err = flags.GetString("ostree-hash"); err != nil {
		return opts, err
	}
	apps, err := flags.GetStringSlice("apps")
	if err != nil {
		return opts, err
	}
	for _, pair := range apps {
		name, hash, ok := strings.Cut(pair, "=")
		if !ok {
			return opts, fmt.Errorf("invalid --apps value '%s', expected name=sha256", pair)
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return opts, fmt.Errorf("invalid --apps value '%s', expected name=sha256", pair)
		}
		if opts.Apps == nil {
			opts.Apps = make(map[string]string)
		}
		opts.Apps[name] = strings.TrimSpace(hash)
	}
	return opts, nil
}

func createUpdate(updates api.UpdatesApi, tag, updateName, path string, opts api.CreateUpdateOptions) error {
	if stat, err := os.Stat(path); err != nil {
		return fmt.Errorf("failed to stat directory '%s': %w", path, err)
	} else if !stat.Mode().IsDir() {
		return fmt.Errorf("a '%s' is neither a directory nor a symlink to a directory", path)
	}

	if opts.HardwareId == "" {
		if _, err := os.Stat(filepath.Join(path, "ostree_repo")); err != nil {
			return errors.New("hardware-id must be specified when uploading an update without an `ostree_repo` directory")
		}
	}

	progress, sourcer := subcommands.TarProgress(subcommands.ArchiveSourcer(path))
	reader := subcommands.GzipStream(progress.StreamWriter(subcommands.TarStream(sourcer)))
	defer reader.Close() //nolint:errcheck

	stop := make(chan bool)
	done := make(chan bool)
	// See cli/subcommands/configs/upload.go for a comment about how reporter works.
	go progress.Report("Uploaded:", stop, done)

	err := updates.CreateUpdate(tag, updateName, opts, reader)
	stop <- err == nil
	<-done
	return err
}
