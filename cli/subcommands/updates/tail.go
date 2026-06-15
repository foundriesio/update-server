// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package updates

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/foundriesio/update-server/cli/api"
	"github.com/spf13/cobra"
)

var tailCmd = &cobra.Command{
	Use:   "tail <tag> <update-name>",
	Short: "Tail update logs",
	Long:  `Follow server-side events for an update or specific rollout`,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		api := api.CtxGetApi(cmd.Context())
		rollout, _ := cmd.Flags().GetString("rollout")
		cobra.CheckErr(tailUpdate(cmd, api.Updates(), args[0], args[1], rollout))
		return nil
	},
}

func init() {
	UpdatesCmd.AddCommand(tailCmd)
	tailCmd.Flags().String("rollout", "", "Specific rollout to tail (optional)")
}

func tailUpdate(cmd *cobra.Command, updates api.UpdatesApi, tag, updateName, rollout string) error {
	var fd io.ReadCloser
	var err error

	if rollout != "" {
		fd, err = updates.TailRollout(tag, updateName, rollout)
		fmt.Printf("Tailing rollout '%s' for update %s/%s\n", rollout, tag, updateName)
	} else {
		fd, err = updates.Tail(tag, updateName)
		fmt.Printf("Tailing all rollouts for update %s/%s\n", tag, updateName)
	}
	cobra.CheckErr(err)
	fmt.Println("Press Ctrl+C to stop...")

	defer func() {
		if err := fd.Close(); err != nil {
			fmt.Printf("warning: failed to close response body: %v\n", err)
		}
	}()

	scanner := bufio.NewScanner(fd)
	var eventType, data string

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			// Empty line marks end of event
			if eventType == "log" && data != "" {
				fmt.Println(data)
			} else if eventType == "error" && data != "" {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "ERROR: %s\n", data)
			}
			eventType = ""
			data = ""
			continue
		}

		if after, ok := strings.CutPrefix(line, "event: "); ok {
			eventType = after
		} else if after, ok := strings.CutPrefix(line, "data: "); ok {
			data = after
		}
		// Ignore id and retry fields
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		return fmt.Errorf("error reading stream: %w", err)
	}

	return nil
}
