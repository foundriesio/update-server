// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package i18n

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLinguasSkipsCommentsAndBlanksAndMapsUnderscore(t *testing.T) {
	t.Parallel()

	src := "# pilot set\n\nen\n  pt_BR  \n# trailing comment\nit\n"

	tags, err := parseLinguas(src)
	require.NoError(t, err)

	got := make([]string, 0, len(tags))
	for _, tag := range tags {
		got = append(got, tag.String())
	}

	assert.Equal(t, []string{"en", "pt-BR", "it"}, got)
}

func TestParseLinguasRejectsInvalidTag(t *testing.T) {
	t.Parallel()

	_, err := parseLinguas("en\nnot a tag!\n")
	require.Error(t, err)
}

func TestSupportedStartsWithEnglish(t *testing.T) {
	t.Parallel()

	tags := Supported()
	require.NotEmpty(t, tags)
	assert.Equal(t, "en", tags[0].String())
}

func TestSupportedReturnsCopy(t *testing.T) {
	t.Parallel()

	tags := Supported()
	tags[0] = tags[1]

	assert.Equal(t, "en", Supported()[0].String())
}

func TestLanguagesReturnsFreshSlice(t *testing.T) {
	t.Parallel()

	langs := Languages()
	langs[0].Name = "changed"

	assert.Equal(t, "English", Languages()[0].Name)
}

func TestLanguagesFollowsSupportedOrder(t *testing.T) {
	t.Parallel()

	langs := Languages()
	tags := Supported()
	require.Len(t, langs, len(tags))

	for i, tag := range tags {
		assert.Equal(t, tag.String(), langs[i].Tag)
	}
}

func TestParseLinguasRejectsMalformedFiles(t *testing.T) {
	t.Parallel()

	tests := []struct{ name, src string }{
		{"bad syntax", "en\n!!\n"},
		{"empty", ""},
		{"comment only", "# nothing here\n"},
		{"first tag not english", "it\nen\n"},
		{"duplicate tag", "en\nit\nit\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := parseLinguas(tt.src)
			assert.Error(t, err)
		})
	}
}

func TestMustParseLinguasPanicsOnMalformedFile(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() { mustParseLinguas("it\nen\n") })
	assert.Panics(t, func() { mustParseLinguas("en\nit\nit\n") })
}
