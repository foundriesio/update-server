// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package i18n

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLanguagesHasEntryForEverySupportedTag(t *testing.T) {
	t.Parallel()

	byTag := make(map[string]Language)
	for _, l := range Languages() {
		byTag[l.Tag] = l
	}

	for _, tag := range Supported() {
		l, ok := byTag[tag.String()]
		assert.True(t, ok, "no Languages entry for LINGUAS tag %q", tag.String())
		assert.NotEmpty(t, l.Name, "empty Name for %q", tag.String())
		assert.NotEmpty(t, l.Dir, "empty Dir for %q", tag.String())
	}
}
