// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

// Package i18n resolves the UI locale and lists the supported locales.
package i18n

import (
	_ "embed"
	"errors"
	"fmt"
	"slices"
	"strings"

	"golang.org/x/text/language"
)

//go:embed LINGUAS
var linguasSrc string

var supported = mustParseLinguas(linguasSrc)

// Supported returns the embedded LINGUAS tags in file order.
func Supported() []language.Tag {
	return slices.Clone(supported)
}

// A malformed embedded file is a build-time mistake, so failing at init is intended.
func mustParseLinguas(src string) []language.Tag {
	tags, err := parseLinguas(src)
	if err != nil {
		panic(err)
	}

	return tags
}

// Accepts POSIX-style `_` separators (pt_BR) as gettext LINGUAS files do.
func parseLinguas(src string) ([]language.Tag, error) {
	var tags []language.Tag

	seen := make(map[string]bool)

	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		tag, err := language.Parse(strings.ReplaceAll(line, "_", "-"))
		if err != nil {
			return nil, fmt.Errorf("LINGUAS: %q: %w", line, err)
		}

		if seen[tag.String()] {
			return nil, fmt.Errorf("LINGUAS: duplicate tag %q", line)
		}

		seen[tag.String()] = true
		tags = append(tags, tag)
	}

	if len(tags) == 0 {
		return nil, errors.New("LINGUAS: no tags")
	}

	// Resolve falls back to en and Languages lists it first, so the file must lead with it.
	if tags[0] != language.English {
		return nil, fmt.Errorf("LINGUAS: first tag must be en, got %q", tags[0])
	}

	return tags, nil
}
