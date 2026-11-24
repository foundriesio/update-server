// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "regenerate oracle-derived testdata (needs xgettext 1.0)")

// requireOracle skips when GNU gettext 1.0 is missing, and fails under -update.
func requireOracle(tb testing.TB) {
	tb.Helper()

	unavailable := tb.Skip
	if *update {
		unavailable = tb.Fatal
	}

	out, err := exec.Command("xgettext", "--version").Output()
	if err != nil {
		unavailable("xgettext not installed; oracle tests need GNU gettext 1.0")
		return
	}

	first, _, _ := strings.Cut(string(out), "\n")
	fields := strings.Fields(first)

	major, err := strconv.Atoi(strings.SplitN(fields[len(fields)-1], ".", 2)[0])
	if err != nil || major < 1 {
		unavailable(fmt.Sprintf("xgettext %q is older than 1.0", first))
		return
	}

	if _, err := exec.LookPath("msgcat"); err != nil {
		unavailable("msgcat not installed")
	}
}

func oraclePOT(tb testing.TB, msgs []Message) []byte {
	tb.Helper()

	got, err := writeXgettext(msgs)
	require.NoError(tb, err)

	return got
}

func readTSV(tb testing.TB, path string) [][]string {
	tb.Helper()

	b, err := os.ReadFile(path)
	require.NoError(tb, err)

	var rows [][]string

	for _, l := range strings.Split(string(b), "\n") {
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}

		rows = append(rows, strings.Split(l, "\t"))
	}

	return rows
}

func refs(file string, lines ...int) []Ref {
	out := make([]Ref, len(lines))
	for i, l := range lines {
		out[i] = Ref{File: file, Line: l}
	}

	return out
}

func TestOracleHarness(t *testing.T) {
	requireOracle(t)

	got := oraclePOT(t, []Message{{ID: "Save", Refs: refs("a/b.html", 3)}})

	want := potHeader + "\n#: a/b.html:3\nmsgid \"Save\"\nmsgstr \"\"\n"
	assert.Equal(t, want, string(got))
}
