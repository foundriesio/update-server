// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const (
	extractorNative   = "native"
	extractorXgettext = "xgettext"
)

func shadowStmt(m Message) string {
	q := strconv.Quote

	switch {
	case m.Ctx != "" && m.Plural != "":
		return fmt.Sprintf("GetNC(%s, %s, 1, %s)", q(m.ID), q(m.Plural), q(m.Ctx))
	case m.Ctx != "":
		return fmt.Sprintf("GetC(%s, %s)", q(m.ID), q(m.Ctx))
	case m.Plural != "":
		return fmt.Sprintf("GetN(%s, %s, 1)", q(m.ID), q(m.Plural))
	}

	return fmt.Sprintf("Get(%s)", q(m.ID))
}

// writeShadow writes one file per referenced path under dir with each call on
// its original line, so xgettext reports the same references as the scanner.
func writeShadow(dir string, msgs []Message) ([]string, error) {
	type site struct {
		file string
		line int
	}

	stmts := map[site][]string{}
	last := map[string]int{}

	for _, m := range msgs {
		if len(m.Refs) == 0 {
			return nil, fmt.Errorf("%q has no source position", m.ID)
		}

		for _, r := range m.Refs {
			if !filepath.IsLocal(filepath.FromSlash(r.File)) {
				return nil, fmt.Errorf("%s: path is outside the scan root", r)
			}

			s := site{r.File, r.Line}
			stmts[s] = append(stmts[s], shadowStmt(m))
			last[r.File] = max(last[r.File], r.Line)
		}
	}

	var files []string

	for file, n := range last {
		lines := make([]string, n)
		for l := 1; l <= n; l++ {
			lines[l-1] = strings.Join(stmts[site{file, l}], "; ")
		}

		path := filepath.Join(dir, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}

		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
			return nil, err
		}

		files = append(files, file)
	}

	slices.Sort(files)

	return files, nil
}

// runXgettext returns xgettext output for files under dir, sorted by msgcat.
func runXgettext(dir string, files []string) ([]byte, error) {
	args := []string{
		"--language=Go", "--from-code=UTF-8", "--no-wrap", "--add-comments=TRANSLATORS:",
		"--keyword=Get", "--keyword=GetN:1,2", "--keyword=GetC:1,2c", "--keyword=GetNC:1,2,4c",
		"--output=-",
	}
	env := append(os.Environ(), "LC_ALL=C")

	var xerr bytes.Buffer

	x := exec.Command("xgettext", append(args, files...)...) //nolint:gosec // G204: fixed binary, scanned file names
	x.Dir, x.Env, x.Stderr = dir, env, &xerr

	raw, err := x.Output()
	if err != nil {
		return nil, fmt.Errorf("xgettext: %w: %s", err, xerr.String())
	}

	var cerr bytes.Buffer

	c := exec.Command("msgcat", "--no-wrap", "--sort-output", "-")
	c.Env, c.Stdin, c.Stderr = env, bytes.NewReader(raw), &cerr

	out, err := c.Output()
	if err != nil {
		return nil, fmt.Errorf("msgcat: %w: %s", err, cerr.String())
	}

	return out, nil
}

// spliceHeader swaps xgettext's header entry, which carries a creation date,
// for ours. xgettext --omit-header drops non-ASCII text, so the header stays.
func spliceHeader(raw []byte) []byte {
	_, rest, found := bytes.Cut(raw, []byte("\n\n"))
	if !found {
		return []byte(potHeader)
	}

	return append([]byte(potHeader+"\n"), rest...)
}

// writeXgettext is the fallback to writePOT: GNU xgettext 1.0 writes the POT
// from shadow Go files holding the scanned messages.
func writeXgettext(msgs []Message) (pot []byte, err error) {
	for _, m := range msgs {
		if len(m.Comments) > 0 {
			return nil, fmt.Errorf("%s: translator comments are not supported by the xgettext extractor", m.where())
		}
	}

	dir, err := os.MkdirTemp("", "i18nscan-")
	if err != nil {
		return nil, err
	}

	defer func() { err = errors.Join(err, os.RemoveAll(dir)) }()

	files, err := writeShadow(dir, msgs)
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return []byte(potHeader), nil
	}

	raw, err := runXgettext(dir, files)
	if err != nil {
		return nil, err
	}

	return spliceHeader(raw), nil
}
