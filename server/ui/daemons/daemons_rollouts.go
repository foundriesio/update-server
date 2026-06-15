// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package daemons

import (
	"errors"
	"os"
	"time"

	"github.com/foundriesio/update-server/context"
)

// WithRolloverInterval sets the rollout rollover interval
func WithRolloverInterval(interval time.Duration) Option {
	return func(d *daemons) {
		d.rolloutOptions.interval = interval
	}
}

type rolloutOptions struct {
	interval time.Duration
}

func (d *daemons) rolloutWatchdog(isProd bool) daemonFunc {
	// Watch for a file once every 5 minutes.
	// API handlers have 5 minutes to write to the file after it was moved.
	// That is more than enough for any in-flight writes to get to the disk.
	return func(stop chan bool) {
		log := context.CtxGetLog(d.context)
		firstRun := true
		for {
			processed := d.processJournal(isProd)
			if firstRun {
				// Do not rollover the journal on application startup - it may have new entries after being processed.
				firstRun = false
			} else if processed {
				if err := d.storage.RolloverRolloutJournal(isProd); err != nil {
					log.Error("failed to roll over the rollout journal", "error", err)
				}
			}
			// Wait between the rollover and processing, so that any in-flight requests finish writing to journal.
			select {
			case <-stop:
				return
			case <-time.After(d.rolloutOptions.interval):
			}
		}
	}
}

func (d *daemons) processJournal(isProd bool) (success bool) {
	log := context.CtxGetLog(d.context)
	success = true
	for line, err := range d.storage.ReadRolloutJournal(isProd) {
		if err != nil {
			// Any journal reading error is critical - return and let the daemon retry later.
			log.Error("failed to read rollout journal", "error", err)
			success = false
			break
		}
		tag := line[0]
		updateName := line[1]
		rolloutName := line[2]
		if rollout, err := d.storage.GetRollout(tag, updateName, rolloutName, isProd); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				log.Warn("rollout file not exist - skipping stale journal entry", "path", line, "is-prod", isProd)
				continue
			}
			// Rollout reading errors are non-critical - log and process other rollouts.
			// Still, a failed rollout journal will be retried later again.
			// User is expected to monitor these errors and investigate the root cause.
			log.Error("failed to process rollout file", "error", err, "path", line, "is-prod", isProd)
			success = false
		} else if !rollout.Commit {
			// Rollout file present but not committed - commit it now.
			if err = d.storage.CommitRollout(tag, updateName, rolloutName, isProd, rollout); err != nil {
				log.Error("failed to commit rollout", "error", err, "path", line, "is-prod", isProd)
				success = false
			}
		}
	}
	return
}
