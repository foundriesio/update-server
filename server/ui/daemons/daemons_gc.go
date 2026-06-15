// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package daemons

import (
	"time"

	"github.com/foundriesio/update-server/storage/users"
)

func userGcDaemonFunc(users *users.Storage) daemonFunc {
	return func(stop chan bool) {
		for {
			select {
			case <-stop:
				return
			case <-time.After(time.Minute * 5):
				users.RunGc()
			}
		}
	}
}
