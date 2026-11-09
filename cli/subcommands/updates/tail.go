// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package updates

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/foundriesio/update-server/cli/api"
	"github.com/spf13/cobra"
)

var tailCmd = &cobra.Command{
	Use:   "tail <update-name>",
	Short: "Tail update logs",
	Long: `Follow server-side events for an update or specific rollout.

With --until-done, stop once every device in the rollout has finished
installing, and exit with an error if any device failed or rolled back,
or if the timeout passes first. Scripts can use it to sequence staged
rollouts.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		api := api.CtxGetApi(cmd.Context())
		rollout, _ := cmd.Flags().GetString("rollout")
		untilDone, _ := cmd.Flags().GetBool("until-done")
		if untilDone {
			if rollout == "" {
				cobra.CheckErr(errors.New("--until-done requires --rollout"))
			}
			timeout, _ := cmd.Flags().GetDuration("timeout")
			interval, _ := cmd.Flags().GetDuration("interval")
			cobra.CheckErr(tailUntilDone(cmd, api.Updates(), args[0], rollout, timeout, interval))
			return nil
		}
		cobra.CheckErr(tailUpdate(cmd, api.Updates(), args[0], rollout))
		return nil
	},
}

func init() {
	UpdatesCmd.AddCommand(tailCmd)
	tailCmd.Flags().String("rollout", "", "Specific rollout to tail (optional)")
	tailCmd.Flags().Bool("until-done", false, "Stop when every device in the rollout finished installing")
	tailCmd.Flags().Duration("timeout", 30*time.Minute, "With --until-done, give up after this long")
	tailCmd.Flags().Duration("interval", 10*time.Second, "With --until-done, time between status checks")
}

func tailUpdate(cmd *cobra.Command, updates api.UpdatesApi, updateName, rollout string) error {
	var fd io.ReadCloser
	var err error

	if rollout != "" {
		fd, err = updates.TailRollout(updateName, rollout)
		fmt.Printf("Tailing rollout '%s' for update %s\n", rollout, updateName)
	} else {
		fd, err = updates.Tail(updateName)
		fmt.Printf("Tailing all rollouts for update %s\n", updateName)
	}
	cobra.CheckErr(err)
	fmt.Println("Press Ctrl+C to stop...")

	defer func() {
		if err := fd.Close(); err != nil {
			fmt.Printf("warning: failed to close response body: %v\n", err)
		}
	}()

	return printEvents(cmd, fd)
}

// tailUntilDone prints a rollout's events until waitRollout sees every
// device finish, then closes the stream.
func tailUntilDone(cmd *cobra.Command, updates api.UpdatesApi, updateName, rollout string, timeout, interval time.Duration) error {
	fd, err := updates.TailRollout(updateName, rollout)
	if err != nil {
		return err
	}
	fmt.Printf("Tailing rollout '%s' for update %s until every device finishes\n", rollout, updateName)

	streamDone := make(chan struct{})
	go func() {
		// Closing the body below ends the stream, so its read error is expected.
		_ = printEvents(cmd, fd)
		close(streamDone)
	}()

	result := waitRollout(updates, updateName, rollout, timeout, interval)
	_ = fd.Close()
	<-streamDone
	return result
}

func printEvents(cmd *cobra.Command, fd io.Reader) error {
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

	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("error reading stream: %w", err)
	}

	return nil
}
