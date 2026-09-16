// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package daemons

import (
	"time"

	"github.com/foundriesio/update-server/context"
)

// WithCertGcInterval sets how often expired certificate records are deleted.
func WithCertGcInterval(interval time.Duration) Option {
	return func(d *daemons) {
		d.certGcOptions.interval = interval
	}
}

type certGcOptions struct {
	interval time.Duration
}

func (d *daemons) certGcDaemon() daemonFunc {
	return func(stop chan bool) {
		log := context.CtxGetLog(d.context)
		for {
			if err := d.storage.DeleteExpiredCerts(time.Now().Unix()); err != nil {
				log.Error("failed to delete expired device certificates", "error", err)
			}
			select {
			case <-stop:
				return
			case <-time.After(d.certGcOptions.interval):
			}
		}
	}
}
