// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const goldenPOT = "testdata/golden/ui.pot.golden"

func TestGolden(t *testing.T) {
	if *update {
		requireOracle(t)

		msgs, err := collect("testdata/golden", "testdata/golden/templates", "testdata/golden/go")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(goldenPOT, oraclePOT(t, msgs), 0o600))
	}

	got, err := generate("testdata/golden", "testdata/golden/templates", "testdata/golden/go")
	require.NoError(t, err)

	want, err := os.ReadFile(goldenPOT)
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got))
}

func TestRealTreeOracle(t *testing.T) {
	requireOracle(t)

	const root = "../../.."

	msgs, err := collect(root, root+"/server/ui/web/templates", root+"/auth", root+"/server/ui/web")
	require.NoError(t, err)

	merged, err := merge(msgs)
	require.NoError(t, err)

	got, err := writePOT(merged)
	require.NoError(t, err)
	assert.Equal(t, string(oraclePOT(t, msgs)), string(got))
}
