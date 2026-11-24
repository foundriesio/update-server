// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tmsg(id string, line int) Message {
	return Message{ID: id, Refs: refs("t.html", line)}
}

func tplural(id, plural string, line int) Message {
	return Message{ID: id, Plural: plural, Refs: refs("t.html", line)}
}

func TestScanText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []Message
	}{
		{"escaped quotes decode", `{{.L.T "Say \"hi\""}}`, []Message{tmsg(`Say "hi"`, 1)}},
		{"raw string keeps backslash", "{{.L.T `Raw \\n %s`}}", []Message{tmsg(`Raw \n %s`, 1)}},
		{"plural pair", `{{.L.N "%[1]s device" "%[1]s devices" .Count}}`, []Message{tplural("%[1]s device", "%[1]s devices", 1)}},
		{"multi-line action", "{{.L.T\n\t\"Save\"}}", []Message{tmsg("Save", 1)}},
		{"tag and dir ignored", `{{.L.Tag}} {{.L.Dir}}`, nil},
		{"field with L suffix ignored", `{{.Foo.L.T "x"}}`, nil},
		{"prose outside action ignored", `<p>Use .L.T "x" here</p>`, nil},
		{"trim markers", `{{- .L.T "x" -}}`, []Message{tmsg("x", 1)}},
		{"dollar root inside range", `{{range .Items}}{{$.L.T "x"}}{{end}}`, []Message{tmsg("x", 1)}},
		{"comment action ignored", `{{/* .L.T "x" */}}`, nil},
		{"delimiter inside string", `{{ .L.T "a }} b" }}`, []Message{tmsg("a }} b", 1)}},
		{"both branches of if", `{{if .Show}}{{.L.T "a"}}{{else}}{{.L.T "b"}}{{end}}`, []Message{tmsg("a", 1), tmsg("b", 1)}},
		{"call nested in parentheses", `{{printf "%s" (.L.T "x")}}`, []Message{tmsg("x", 1)}},
		{"line numbers", "x\n{{.L.T \"a\"}}\n\n{{.L.T \"b\"}}", []Message{tmsg("a", 2), tmsg("b", 4)}},
		{"crlf line endings", "x\r\n{{.L.T \"a\"}}\r\n{{.L.T \"b\"}}", []Message{tmsg("a", 2), tmsg("b", 3)}},
		{"identifier line of a multi-line action", "\n{{.L.T\n\"a\"}}", []Message{tmsg("a", 2)}},
		{"else list of range and with", "{{range .A}}{{else}}{{.L.T \"a\"}}{{end}}\n{{with .B}}{{else}}{{.L.T \"b\"}}{{end}}", []Message{tmsg("a", 1), tmsg("b", 2)}},
		{"chain on parenthesized call", `{{(.L.T "x").Foo}}`, []Message{tmsg("x", 1)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := scanText("t.html", tc.in)
			require.NoError(t, err)

			if len(tc.want) == 0 {
				assert.Empty(t, got)
				return
			}

			assert.Equal(t, tc.want, got)
		})
	}
}

func TestScanTextDefineBlock(t *testing.T) {
	in := "{{define \"d\"}}{{.L.T \"a\"}}{{end}}\n{{block \"b\" .}}{{.L.T \"b\"}}{{end}}"

	got, err := scanText("t.html", in)
	require.NoError(t, err)
	assert.ElementsMatch(t, []Message{tmsg("a", 1), tmsg("b", 2)}, got)
}

func TestScanTextRejectsNonLiteral(t *testing.T) {
	tests := map[string]string{
		"variable message":        `{{.L.T .Msg}}`,
		"plural missing plural":   `{{.L.N "a" .Count}}`,
		"plural non-literal":      `{{.L.N "a" .Other .Count}}`,
		"message missing":         `{{.L.T}}`,
		"piped message missing":   `{{ "x" | .L.T }}`,
		"printf message":          `{{ .L.T (printf "x") }}`,
		"dollar root non-literal": `{{ $.L.T .Msg }}`,
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := scanText("page.html", in)
			require.Error(t, err)
			assert.ErrorContains(t, err, "page.html:1")
		})
	}
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
}

func TestGenerateMergesAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.html", `{{.L.T "Save"}}`)
	writeFile(t, dir, "b.html", `{{.L.T "Save"}}`)

	got, err := generate(dir, dir)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(got), `msgid "Save"`))
	assert.Contains(t, string(got), "#: a.html:1 b.html:1\n")
}

func TestGenerateScansNestedDirs(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "sub", "deep")
	require.NoError(t, os.MkdirAll(nested, 0o700))
	writeFile(t, nested, "page.html", `{{.L.T "Nested"}}`)

	got, err := generate(dir, dir)
	require.NoError(t, err)
	assert.Contains(t, string(got), "#: sub/deep/page.html:1\nmsgid \"Nested\"")
}

func TestGenerateReportsAllBadFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.html", `{{.L.T .Msg}}`)
	writeFile(t, dir, "b.html", `{{.L.N "a" .Count}}`)

	_, err := generate(dir, dir)
	require.Error(t, err)
	assert.ErrorContains(t, err, "a.html:1")
	assert.ErrorContains(t, err, "b.html:1")
}

func TestGenerateFailsWithoutOutputOnBadCall(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "page.html", `{{.L.T .Msg}}`)

	got, err := generate(dir, dir)
	require.Error(t, err)
	assert.Nil(t, got)
}

func TestGenerateEmptyIsHeaderOnly(t *testing.T) {
	dir := t.TempDir()

	got, err := generate(dir, dir)
	require.NoError(t, err)
	assert.Equal(t, potHeader, string(got))
}

func TestGenerateRejectsPluralConflict(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.html", `{{.L.T "x"}}`)
	writeFile(t, dir, "b.html", `{{.L.N "x" "xs" .N}}`)

	_, err := generate(dir, dir)
	require.Error(t, err)
	assert.ErrorContains(t, err, "a.html:1")
	assert.ErrorContains(t, err, "b.html:1")
}

func TestGenerateRejectsPluralFormatMismatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.html", `{{.L.N "one" "many %d" .N}}`)

	_, err := generate(dir, dir)
	require.Error(t, err)
	assert.ErrorContains(t, err, "a.html:1")
	assert.ErrorContains(t, err, "argument 1")
}

func TestGenerateDeterministic(t *testing.T) {
	dir := t.TempDir()

	for _, n := range []string{"a", "b", "c", "d", "e", "f"} {
		writeFile(t, dir, n+".html", `{{.L.T "Same"}}{{.L.T "`+n+`"}}`)
	}

	first, err := generate(dir, dir)
	require.NoError(t, err)

	for range 5 {
		again, err := generate(dir, dir)
		require.NoError(t, err)
		assert.Equal(t, string(first), string(again))
	}
}

func TestScanGoFindsLiteralCalls(t *testing.T) {
	dir := t.TempDir()
	src := `package x

func f(p printer) {
	p.T("Devices")
	p.N("%[1]s device", "%[1]s devices", 2)
}
`
	writeFile(t, dir, "a.go", src)

	got, err := scanGo(dir, dir)
	require.NoError(t, err)
	assert.Equal(t, []Message{
		{ID: "Devices", Refs: refs("a.go", 4)},
		{ID: "%[1]s device", Plural: "%[1]s devices", Refs: refs("a.go", 5)},
	}, got)
}

func TestScanGoRejectsNonLiteralMessage(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "package x\n\nfunc f(p printer, k string) { p.T(k) }\n")

	_, err := scanGo(dir, dir)
	assert.ErrorContains(t, err, "not a string literal")
}

func TestScanGoRejectsMissingArgs(t *testing.T) {
	tests := map[string]string{
		"T without arguments":   "p.T()",
		"N without arguments":   "p.N()",
		"N without plural form": `p.N("a")`,
	}
	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "a.go", "package x\n\nfunc f(p printer) { "+call+" }\n")

			got, err := scanGo(dir, dir)
			require.Error(t, err)
			assert.Nil(t, got)
			assert.ErrorContains(t, err, "a.go:3:")
			assert.ErrorContains(t, err, "missing message string literal")
		})
	}
}

func TestScanGoReportsAllBadFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "package x\n\nfunc f(p printer, k string) { p.T(k) }\n")
	writeFile(t, dir, "b.go", "package x\n\nfunc f(p printer) { p.T() }\n")

	_, err := scanGo(dir, dir)
	require.Error(t, err)
	assert.ErrorContains(t, err, "a.go:3:")
	assert.ErrorContains(t, err, "b.go:3:")
}

func TestScanGoErrorUsesRelPath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "package x\n\nfunc f(p printer, k string) { p.T(k) }\n")

	_, err := scanGo(dir, dir)
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "a.go:3:"), err.Error())
	assert.NotContains(t, err.Error(), dir)
}

func TestScanGoSkipsTestFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a_test.go", "package x\n\nfunc f(p printer) { p.T(\"hidden\") }\n")

	got, err := scanGo(dir, dir)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestScanGoNestedRelRef(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(nested, 0o700))
	writeFile(t, nested, "a.go", "package x\n\nfunc f(p printer) { p.T(\"Deep\") }\n")

	got, err := scanGo(dir, dir)
	require.NoError(t, err)
	assert.Equal(t, []Message{{ID: "Deep", Refs: refs("sub/a.go", 3)}}, got)
}

func TestGenerateMergesGoAndTemplate(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.html", `{{.L.T "Save"}}`)
	writeFile(t, dir, "a.go", "package x\n\nfunc f(p printer) { p.T(\"Save\") }\n")

	got, err := generate(dir, dir, dir)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(got), `msgid "Save"`))
	assert.Contains(t, string(got), "#: a.go:3 a.html:1\n")
}

func TestGenerateGoPluralConflict(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.html", `{{.L.T "x"}}`)
	writeFile(t, dir, "a.go", "package x\n\nfunc f(p printer) { p.N(\"x\", \"xs\", 1) }\n")

	_, err := generate(dir, dir, dir)
	require.Error(t, err)
	assert.ErrorContains(t, err, "a.go:3")
	assert.ErrorContains(t, err, "a.html:1")
}

func TestGenerateTwoGoDirs(t *testing.T) {
	root := t.TempDir()
	tpl := filepath.Join(root, "tpl")
	one := filepath.Join(root, "one")
	two := filepath.Join(root, "two")

	for _, d := range []string{tpl, one, two} {
		require.NoError(t, os.MkdirAll(d, 0o700))
	}

	writeFile(t, one, "a.go", "package x\n\nfunc f(p printer) { p.T(\"One\") }\n")
	writeFile(t, two, "b.go", "package x\n\nfunc f(p printer) { p.T(\"Two\") }\n")

	got, err := generate(root, tpl, one, two)
	require.NoError(t, err)
	assert.Contains(t, string(got), "#: one/a.go:3\nmsgid \"One\"")
	assert.Contains(t, string(got), "#: two/b.go:3\nmsgid \"Two\"")
}

func TestSplitDirs(t *testing.T) {
	assert.Nil(t, splitDirs(""))
	assert.Equal(t, []string{"a"}, splitDirs("a,"))
	assert.Equal(t, []string{"a", "b"}, splitDirs(" a , ,b,"))
}
