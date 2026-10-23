// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package version

import "time"

var Version string

// startTime stands in for Version in dev builds (go run/go test), where
// -ldflags isn't applied and Version is empty, so asset cache-busting still
// changes across restarts.
var startTime = time.Now().Format("20060102150405")

// AssetVersion returns a token for cache-busting static asset URLs: the
// build version when set, otherwise the process start time.
func AssetVersion() string {
	if Version != "" {
		return Version
	}
	return startTime
}
