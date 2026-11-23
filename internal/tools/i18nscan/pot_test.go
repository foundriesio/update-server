// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWritePOTLayout(t *testing.T) {
	got, err := writePOT([]Message{
		{ID: "b", Refs: refs("x.html", 2)},
		{ID: "%[1]s day", Plural: "%[1]s days", Refs: refs("x.html", 1)},
		{Ctx: "menu", ID: "a", Comments: []string{"TRANSLATORS: verb"}, Refs: refs("y.go", 7)},
	})
	require.NoError(t, err)

	want := potHeader + `
#: x.html:1
#, go-format
msgid "%[1]s day"
msgid_plural "%[1]s days"
msgstr[0] ""
msgstr[1] ""

#. TRANSLATORS: verb
#: y.go:7
msgctxt "menu"
msgid "a"
msgstr ""

#: x.html:2
msgid "b"
msgstr ""
`
	assert.Equal(t, want, string(got))
}

func TestWritePOTEmptyCommentLine(t *testing.T) {
	got, err := writePOT([]Message{{ID: "a", Comments: []string{"TRANSLATORS: a", "", "b"}}})
	require.NoError(t, err)
	assert.Contains(t, string(got), "\n#. TRANSLATORS: a\n#.\n#. b\nmsgid")
}

func TestWritePOTEmptyIsHeaderOnly(t *testing.T) {
	got, err := writePOT(nil)
	require.NoError(t, err)
	assert.Equal(t, potHeader, string(got))
}

func TestWritePOTRejectsSpacePath(t *testing.T) {
	_, err := writePOT([]Message{{ID: "a", Refs: refs("my dir/x.html", 1)}})
	require.Error(t, err)
	assert.ErrorContains(t, err, "my dir/x.html:1")
}

func TestWritePOTSortsBytewise(t *testing.T) {
	got, err := writePOT([]Message{{ID: "b"}, {ID: "B"}, {ID: "a"}, {ID: "é"}, {ID: "e"}})
	require.NoError(t, err)

	var ids []string

	for _, l := range strings.Split(string(got), "\n") {
		if id, ok := strings.CutPrefix(l, "msgid \""); ok && id != `"` {
			ids = append(ids, strings.TrimSuffix(id, `"`))
		}
	}

	assert.Equal(t, []string{"B", "a", "b", "e", "é"}, ids)
}

func TestPOTMatchesOracle(t *testing.T) {
	requireOracle(t)

	var many []int
	for i := 1; i <= 30; i++ {
		many = append(many, i)
	}

	widthRefs := func(n int) []Ref {
		return append(refs("a.html", 11, 12, 13, 14, 15, 16), refs(strings.Repeat("b", n)+".html", 1)...)
	}

	cases := map[string][]Message{
		"plain":      {{ID: "Save", Refs: refs("a.html", 1)}},
		"plural":     {{ID: "%[1]s day", Plural: "%[1]s days", Refs: refs("a.html", 1)}},
		"context":    {{Ctx: "menu", ID: "Open", Refs: refs("a.html", 1)}},
		"ctx plural": {{Ctx: "c", ID: "%[1]s a", Plural: "%[1]s as", Refs: refs("a.html", 1)}},
		"same id three ways": {
			{ID: "Open", Refs: refs("a.html", 1)},
			{Ctx: "menu", ID: "Open", Refs: refs("a.html", 2)},
			{Ctx: "door", ID: "Open", Refs: refs("a.html", 3)},
		},
		"sort bytes": {
			{ID: "b", Refs: refs("a.html", 1)}, {ID: "B", Refs: refs("a.html", 2)},
			{ID: "a", Refs: refs("a.html", 3)}, {ID: "é", Refs: refs("a.html", 4)},
			{ID: "e", Refs: refs("a.html", 5)}, {ID: "Z", Refs: refs("a.html", 6)},
		},
		"wrapped refs":       {{ID: "Cancel", Refs: refs("server/ui/web/templates/devices_list.html", many...)}},
		"refs line of 78":    {{ID: "W", Refs: widthRefs(8)}},
		"refs line of 79":    {{ID: "W", Refs: widthRefs(9)}},
		"refs line of 80":    {{ID: "W", Refs: widthRefs(10)}},
		"refs 79 bytes utf8": {{ID: "W", Refs: append(refs("a.html", 11, 12, 13, 14, 15, 16), refs("éééé"+"b.html", 1)...)}},
		"refs 80 bytes utf8": {{ID: "W", Refs: append(refs("a.html", 11, 12, 13, 14, 15, 16), refs("éééé"+"bb.html", 1)...)}},
		"single long ref":    {{ID: "W", Refs: refs("a.html", 1, 2, 3, 4)}, {ID: "L", Refs: refs(strings.Repeat("z", 100)+".html", 1, 2)}},
		"refs across files": {{ID: "Close", Refs: append(
			refs("server/ui/web/templates/settings.html", 148, 193),
			refs("auth/provider_local.go", 160)...)}},
		"two messages one line": {
			{ID: "One", Refs: refs("a.html", 5)},
			{ID: "Two", Refs: refs("a.html", 5)},
		},
		"same line twice": {
			{ID: "Dup", Refs: refs("a.html", 5)},
			{ID: "Dup", Refs: refs("a.html", 5)},
		},
		"quote and backslash": {{ID: `Say "hi" \ done`, Refs: refs("a.html", 1)}},
		"tab cr":              {{ID: "tab\there\rcr", Refs: refs("a.html", 1)}},
		"control chars":       {{ID: "bell\a bs\b ff\f vt\v \x01 \x7f", Refs: refs("a.html", 1)}},
		"non ascii":           {{ID: "café · → …", Refs: refs("a.html", 1)}},
		"newline middle":      {{ID: "a\nb", Refs: refs("a.html", 1)}},
		"newline end":         {{ID: "a\n", Refs: refs("a.html", 1)}},
		"newline only":        {{ID: "\n", Refs: refs("a.html", 1)}},
		"blank line middle":   {{ID: "a\n\nb", Refs: refs("a.html", 1)}},
		"newline start":       {{ID: "\na", Refs: refs("a.html", 1)}},
		"plural multi-line":   {{ID: "%[1]s a\nb", Plural: "%[1]s as\nb", Refs: refs("a.html", 1)}},
		"plural no format":    {{ID: "one file", Plural: "many files", Refs: refs("a.html", 1)}},
		"plural invalid fmt":  {{ID: "100% a", Plural: "100% as", Refs: refs("a.html", 1)}},
		"plural d format":     {{ID: "%d file", Plural: "%d files", Refs: refs("a.html", 1)}},
		"plural bad second":   {{ID: "%d one", Plural: "%z many", Refs: refs("a.html", 1)}},
		"plural arg clash":    {{ID: "%[1]d", Plural: "%[1]d %[1]s", Refs: refs("a.html", 1)}},
		"plural pct only":     {{ID: "a", Plural: "%%", Refs: refs("a.html", 1)}},
		"formats": {
			{ID: "%d%% done", Refs: refs("a.html", 1)}, {ID: "100% sure", Refs: refs("a.html", 2)},
			{ID: "%[3]*.[2]*[1]f", Refs: refs("a.html", 3)}, {ID: "bad %z verb", Refs: refs("a.html", 4)},
		},
	}

	for name, msgs := range cases {
		t.Run(name, func(t *testing.T) {
			merged, err := merge(msgs)
			require.NoError(t, err)

			got, err := writePOT(merged)
			require.NoError(t, err)

			assert.Equal(t, string(oraclePOT(t, msgs)), string(got))
		})
	}
}

func FuzzPOTMatchesOracle(f *testing.F) {
	for _, s := range []string{"plain", "a\nb", "q\"q", "\x01", "tab\t", "café", "%d%%", "%[1]s", `\n`, "100% sure"} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		// msgcat, the oracle's sort stage, rejects EOT as a context separator.
		if s == "" || !utf8.ValidString(s) || strings.ContainsAny(s, "\x00\x04") {
			t.Skip()
		}

		requireOracle(t)

		msgs := []Message{{ID: s, Refs: refs("f.html", 1)}}

		got, err := writePOT(msgs)
		require.NoError(t, err)

		assert.Equal(t, string(oraclePOT(t, msgs)), string(got))
	})
}
