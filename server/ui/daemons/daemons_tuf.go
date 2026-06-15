// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package daemons

import (
	"time"

	"github.com/foundriesio/update-server/context"
)

const (
	tufRefreshDefaultInterval = 24 * time.Hour
	tufRefreshThreshold       = 30 * 24 * time.Hour
)

// WithTufRefreshInterval sets the interval at which TUF metadata expiry is checked.
func WithTufRefreshInterval(interval time.Duration) Option {
	return func(d *daemons) {
		d.tufOptions.interval = interval
	}
}

type tufOptions struct {
	interval time.Duration
}

func (d *daemons) tufRefreshDaemon() daemonFunc {
	return func(stop chan bool) {
		log := context.CtxGetLog(d.context)
		for {
			if err := d.storage.RefreshAllTuf(tufRefreshThreshold); err != nil {
				log.Error("failed to refresh TUF metadata", "error", err)
			}
			select {
			case <-stop:
				return
			case <-time.After(d.tufOptions.interval):
			}
		}
	}
}
