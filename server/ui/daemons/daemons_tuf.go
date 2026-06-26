// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package daemons

import (
	"time"

	"github.com/foundriesio/update-server/context"
)

// WithTufRefreshInterval sets how often the TUF refresh daemon scans for
// metadata that is approaching expiry.
func WithTufRefreshInterval(interval time.Duration) Option {
	return func(d *daemons) {
		d.tufOptions.interval = interval
	}
}

type tufOptions struct {
	interval time.Duration
}

// tufRefreshDaemon periodically refreshes the snapshot and timestamp TUF
// metadata of every tag whose metadata is approaching expiry. The timestamp is
// short lived and is refreshed far more often than the snapshot.
func (d *daemons) tufRefreshDaemon() daemonFunc {
	return func(stop chan bool) {
		log := context.CtxGetLog(d.context)
		for {
			if err := d.storage.RefreshTufTimestamps(d.context); err != nil {
				log.Error("failed to refresh TUF metadata expiry", "error", err)
			}
			select {
			case <-stop:
				return
			case <-time.After(d.tufOptions.interval):
			}
		}
	}
}
