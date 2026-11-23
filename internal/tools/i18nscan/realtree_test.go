// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRealTree fails when the committed ui.pot is stale: run make i18n-extract.
func TestRealTree(t *testing.T) {
	const root = "../../.."

	got, err := generate(root, root+"/server/ui/web/templates", root+"/auth", root+"/server/ui/web")
	require.NoError(t, err)

	want, err := os.ReadFile(root + "/server/ui/i18n/po/ui.pot")
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got))
}

// TestRealTreeXgettext checks the xgettext fallback writes the committed ui.pot.
func TestRealTreeXgettext(t *testing.T) {
	requireOracle(t)

	const root = "../../.."

	got, err := generateWith(writeXgettext, root, root+"/server/ui/web/templates", root+"/auth", root+"/server/ui/web")
	require.NoError(t, err)

	want, err := os.ReadFile(root + "/server/ui/i18n/po/ui.pot")
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got))
}

func TestWriteXgettextRejects(t *testing.T) {
	tests := map[string]Message{
		"no position":  {ID: "a"},
		"outside root": {ID: "a", Refs: refs("../x.html", 1)},
		"comment":      {ID: "a", Refs: refs("x.html", 1), Comments: []string{"hint"}},
	}

	for name, m := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := writeXgettext([]Message{m})
			assert.Error(t, err)
		})
	}
}
