// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package updates

import (
	"testing"

	"github.com/stretchr/testify/require"

	storage "github.com/foundriesio/update-server/storage/api"
)

func TestRolloutProgress(t *testing.T) {
	finished, unsuccessful := rolloutProgress(map[string]int{
		"Download started":                 1,
		storage.InstallationSucceededState: 2,
		storage.InstallationFailedState:    1,
		storage.RolledBackState:            1,
		storage.MissingState:               3,
	})
	require.Equal(t, 4, finished)
	require.Equal(t, 2, unsuccessful)

	finished, unsuccessful = rolloutProgress(nil)
	require.Zero(t, finished)
	require.Zero(t, unsuccessful)
}

func TestFormatSummary(t *testing.T) {
	require.Empty(t, formatSummary(nil))
	require.Equal(t, "A: 1, B: 2", formatSummary(map[string]int{"B": 2, "A": 1}))
}
