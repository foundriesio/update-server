// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

// Command i18nscan writes the POT file for the English messages in .L.T and
// .L.N template calls and in Go T and N calls. Its output matches GNU
// xgettext 1.0 for the same messages.
// Only direct calls are extracted: no method values or template aliases such as $l := .L.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	templates := flag.String("templates", "", "directory of *.html templates to scan")
	goDirs := flag.String("go", "", "comma-separated directories of Go sources whose T and N calls are scanned")
	root := flag.String("root", ".", "directory that source paths in #: references are relative to")
	out := flag.String("out", "", "POT file to write")
	extractor := flag.String("extractor", extractorNative,
		"POT writer: native, or xgettext (needs GNU gettext 1.0 on PATH)")
	flag.Parse()

	if *templates == "" || *out == "" {
		flag.Usage()
		return errors.New("-templates and -out are required")
	}

	write := writePOT

	switch *extractor {
	case extractorNative:
	case extractorXgettext:
		write = writeXgettext
	default:
		return fmt.Errorf("unknown -extractor %q", *extractor)
	}

	pot, err := generateWith(write, *root, *templates, splitDirs(*goDirs)...)
	if err != nil {
		return err
	}

	return os.WriteFile(*out, pot, 0o600)
}

func relPath(root, path string) (string, error) {
	r, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}

	return filepath.ToSlash(r), nil
}

// collect returns every message found, unmerged. It fails if any call has a
// non-literal message, so no message is dropped silently.
func collect(root, templates string, goDirs ...string) ([]Message, error) {
	var msgs []Message

	var errs []error

	err := filepath.WalkDir(templates, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || filepath.Ext(path) != ".html" {
			return nil
		}

		text, err := os.ReadFile(path) //nolint:gosec // G304: path comes from walking the -templates tree
		if err != nil {
			return err
		}

		name, err := relPath(root, path)
		if err != nil {
			return err
		}

		found, err := scanText(name, string(text))
		if err != nil {
			errs = append(errs, err)
			return nil
		}

		msgs = append(msgs, found...)

		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	for _, gd := range goDirs {
		found, err := scanGo(root, gd)
		if err != nil {
			return nil, err
		}

		msgs = append(msgs, found...)
	}

	return msgs, nil
}

// generate returns the POT text, or nothing if any message is invalid.
func generate(root, templates string, goDirs ...string) ([]byte, error) {
	return generateWith(writePOT, root, templates, goDirs...)
}

// generateWith is generate with the POT writer chosen by the caller.
func generateWith(write func([]Message) ([]byte, error), root, templates string, goDirs ...string) ([]byte, error) {
	msgs, err := collect(root, templates, goDirs...)
	if err != nil {
		return nil, err
	}

	merged, err := merge(msgs)
	errs := []error{err}

	for _, m := range merged {
		if m.Plural == "" {
			continue
		}

		if err := checkPlural(m.ID, m.Plural); err != nil {
			errs = append(errs, fmt.Errorf("%s: %q: %w", m.where(), m.ID, err))
		}
	}

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	return write(merged)
}

// splitDirs splits a comma-separated list, skipping blank entries.
func splitDirs(s string) []string {
	var dirs []string

	for d := range strings.SplitSeq(s, ",") {
		if d = strings.TrimSpace(d); d != "" {
			dirs = append(dirs, d)
		}
	}

	return dirs
}
