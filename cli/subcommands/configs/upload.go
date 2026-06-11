// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package configs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/foundriesio/update-server/cli/api"
	"github.com/foundriesio/update-server/cli/subcommands"
)

var uploadCmd = &cobra.Command{
	Use:   "upload <configs.tgz> | --dir <configs-dir>",
	Short: "Upload configs",
	Long: `Upload configs to the Update server.

	Supported file formats are .tar, .tar.gz, and .tgz.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		path := args[0]
		isDir, _ := cmd.Flags().GetBool("dir")
		api := api.CtxGetApi(cmd.Context())
		cobra.CheckErr(uploadConfigs(api.Configs(), path, isDir))
	},
	Hidden: true,
}

func init() {
	ConfigsCmd.AddCommand(uploadCmd)
	uploadCmd.Flags().BoolP("dir", "d", false, "Archive a directory with configs to upload")
}

func uploadConfigs(capi api.ConfigsApi, path string, isDir bool) error {
	var (
		reader   io.ReadCloser
		reporter func(string, chan bool, chan bool)
	)

	if isDir {
		if stat, err := os.Stat(path); err != nil {
			return fmt.Errorf("failed to stat directory '%s': %w", path, err)
		} else if !stat.Mode().IsDir() {
			return fmt.Errorf("a '%s' is neither a directory nor a symlink to a directory", path)
		}

		denyNonRegular := func(entry subcommands.ArchiveEntry) error {
			mode := entry.Info.Mode()
			if mode.IsDir() || mode.IsRegular() {
				return nil
			}
			return fmt.Errorf("only regular files and directories are allowed for configs, but '%s' is neither", entry.Path)
		}

		progress, sourcer := subcommands.TarProgress(subcommands.ArchiveSourcer(path, denyNonRegular))
		reporter = progress.Report

		reader = subcommands.GzipStream(progress.StreamWriter(subcommands.TarStream(sourcer)))
		defer reader.Close() //nolint:errcheck
	} else {
		var isGzip bool
		switch ext := filepath.Ext(path); ext {
		case ".tar":
			break
		case ".tar.gz", ".tgz":
			isGzip = true
		default:
			return fmt.Errorf("supported file types are '.tar, .tar.gz, .tgz', but '%s' given", ext)
		}

		fd, err := os.OpenFile(path, os.O_RDONLY, 0)
		if err != nil {
			return fmt.Errorf("failed to read file '%s': %w", path, err)
		}
		defer fd.Close() //nolint:errcheck

		stat, err := fd.Stat()
		if err != nil {
			return fmt.Errorf("failed to read file '%s': %w", path, err)
		} else if !stat.Mode().IsRegular() {
			return fmt.Errorf("a '%s' is neither a regular file nor a symlink to a regular file", path)
		}

		progress := subcommands.FileProgress(stat.Size())
		reporter = progress.Report

		if !isGzip {
			// Gzip raw tar files on-the-fly to save network traffic
			reader = subcommands.GzipStream(progress.StreamWriter(func(gzipper io.Writer) error {
				_, err = io.Copy(gzipper, fd)
				return err
			}))
			defer reader.Close() //nolint:errcheck
		} else {
			reader = progress.StreamReader(fd)
		}
	}

	stop := make(chan bool)
	done := make(chan bool)
	// Reporter is reporting based on the raw (disk) file sizes before compression.
	// Rationale: gzip compression is a part of a transport, not storage.
	// An io.Pipe|Reader|Writer interfaces warrant that a difference between actual bytes read/written is minimal.
	// As per documentation it is up to 32KB, unless we decide to reconfigure default buffers.
	// This gives us an extremely accurate precision, when we focus solely on input data sizes.
	go reporter("Uploaded:", stop, done)

	err := capi.Upload(reader,
		api.HttpHeader("Content-Type", "application/x-tar"),
		api.HttpHeader("Content-Encoding", "gzip"),
	)
	stop <- err == nil
	<-done
	return err
}
