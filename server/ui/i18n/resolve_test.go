// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package i18n

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		stored string
		header string
		want   string
	}{
		{"weighted header picks highest q", "", "it;q=0.9,en;q=0.8", "it"},
		{"weighted header ignores list order", "", "en;q=0.5,it;q=0.9", "it"},
		{"malformed header falls back to en", "", "it;q=abc,,,", "en"},
		{"unsupported only falls back to en", "", "fr", "en"},
		{"empty header falls back to en", "", "", "en"},
		{"region variant resolves to base tag", "", "it-CH", "it"},
		{"empty stored follows header", "", "it", "it"},
		{"supported stored wins over header", "it", "en", "it"},
		{"invalid stored falls through to header", "fr", "it", "it"},
		{"invalid stored and unsupported header fall to en", "fr", "de", "en"},
		{"non-canonical stored falls through to header", "it-IT", "it", "it"},
		{"extension tag returns canonical it", "", "it-u-rg-chzzzz", "it"},
		{"extension on unsupported language", "", "fr-u-rg-chzzzz", "en"},
		{"region of default", "", "en-US", "en"},
		{"q=0 dropped", "", "it;q=0,en", "en"},
		{"malformed value", "", "it;q=abc", "en"},
		{"wildcard after supported", "", "it,*;q=0.1", "it"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, Resolve(tt.stored, tt.header).String())
		})
	}
}

// One malformed entry rejects the whole header, so a valid "it" earlier in it is ignored.
func TestResolveMalformedEntryFallsBackToEnglishForWholeHeader(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "en", Resolve("", "it, xx_garbage;q=0.5").String())
}

// A header over the length cap is treated as absent rather than parsed.
func TestResolveOversizedHeaderFallsBackToEnglish(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "en", Resolve("", strings.Repeat("it,", 700)).String())
}

// Exact match on the canonical supported tag only: non-canonical spellings are
// rejected so a stored value never reaches a catalog lookup in another form.
func TestValidAcceptsOnlyExactSupportedTags(t *testing.T) {
	t.Parallel()

	tag, ok := Valid("it")
	assert.True(t, ok)
	assert.Equal(t, "it", tag.String())

	for _, bad := range []string{"", "fr", "it-IT", "IT", "it_IT", "../it"} {
		_, ok := Valid(bad)
		assert.False(t, ok, "Valid(%q) must be false", bad)
	}
}
