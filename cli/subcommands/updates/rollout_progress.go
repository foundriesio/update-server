// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package updates

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/foundriesio/update-server/cli/api"
	storage "github.com/foundriesio/update-server/storage/api"
)

// rolloutProgress reports how many of the rollout's devices finished and how
// many of those didn't end up on the update.
func rolloutProgress(summary map[string]int) (finished, unsuccessful int) {
	for status, count := range summary {
		switch status {
		case storage.InstallationSucceededState:
			finished += count
		case storage.InstallationFailedState, storage.RolledBackState:
			finished += count
			unsuccessful += count
		}
	}
	return
}

func formatSummary(summary map[string]int) string {
	parts := make([]string, 0, len(summary))
	for _, status := range slices.Sorted(maps.Keys(summary)) {
		parts = append(parts, fmt.Sprintf("%s: %d", status, summary[status]))
	}
	return strings.Join(parts, ", ")
}

func waitRollout(updates api.UpdatesApi, updateName, rollout string, timeout, interval time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		r, err := updates.GetRollout(updateName, rollout)
		if err != nil {
			return err
		}
		summary, err := updates.GetRolloutSummary(updateName, rollout)
		if err != nil {
			return err
		}
		// Effect stays empty until the server has resolved the rollout's devices.
		if total := len(r.Effect); total > 0 {
			finished, unsuccessful := rolloutProgress(summary.Status)
			if finished >= total {
				if unsuccessful > 0 {
					return fmt.Errorf("%d of %d devices did not install update %s", unsuccessful, total, updateName)
				}
				fmt.Printf("All %d devices installed update %s\n", total, updateName)
				return nil
			}
		}
		if time.Now().Add(interval).After(deadline) {
			return fmt.Errorf("timed out after %s waiting for rollout %s (%s)", timeout, rollout, formatSummary(summary.Status))
		}
		time.Sleep(interval)
	}
}
