// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package i18n

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"
)

func TestItalianTranslatesLanguage(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Lingua", For(language.Italian).T("Language"))
}

func TestUnknownMessageReturnsKey(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Not in catalog", For(language.Italian).T("Not in catalog"))
}

func TestBadFormatStringDoesNotPanic(t *testing.T) {
	t.Parallel()

	// A variable keeps go vet's printf check from rejecting the deliberately bad format.
	bad := "%d %s"

	for _, tag := range []language.Tag{language.English, language.Italian} {
		assert.Equal(t, "1 %!s(MISSING)", For(tag).T(bad, 1), "tag %s", tag)
	}
}

func TestNArgOrder(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "3 of 9 devices", For(language.English).N("%[1]s of %[2]s devices", "%[1]s of %[2]s devices", 3, "9"))
}

func TestNCountlessFormIsReported(t *testing.T) {
	t.Parallel()

	// Every form must contain %[1]s; a form without it gets fmt's EXTRA suffix.
	assert.Equal(t, "One update%!(EXTRA string=1)", For(language.English).N("One update", "%[1]s updates", 1))
}

func TestTFormatsArgsPerLocale(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Currently using English", For(language.English).T("Currently using %[1]s", "English"))
	assert.Equal(t, "Attualmente in uso: English", For(language.Italian).T("Currently using %[1]s", "English"))
}

func TestEnglishKeyWithoutArgsIsNotFormatted(t *testing.T) {
	t.Parallel()

	// Spreading nil passes zero args but hides the non-constant format from go vet.
	key := "100% done"

	assert.Equal(t, "100% done", For(language.English).T(key, nil...))
}

func TestNPluralizesPerLocale(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tag  language.Tag
		n    int
		want string
	}{
		{language.English, 0, "0 devices"},
		{language.English, 1, "1 device"},
		{language.English, 2, "2 devices"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/%d", tt.tag, tt.n), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, For(tt.tag).N("%[1]s device", "%[1]s devices", tt.n))
		})
	}
}

func TestForUnknownTagFallsBackToEnglish(t *testing.T) {
	t.Parallel()

	p := For(language.French)
	assert.Equal(t, "en", p.Tag())
	assert.Equal(t, "Language", p.T("Language"))
}

func TestTagAndDir(t *testing.T) {
	t.Parallel()

	require.Equal(t, "it", For(language.Italian).Tag())
	assert.Equal(t, "ltr", For(language.Italian).Dir())
	assert.Equal(t, "en", For(language.English).Tag())
	assert.Equal(t, "ltr", For(language.English).Dir())
}

func TestPrintersConcurrentUse(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup

	for i := range 50 {
		wg.Go(func() {
			p, want := For(language.English), "Language"
			if i%2 == 1 {
				p, want = For(language.Italian), "Lingua"
			}

			assert.Equal(t, want, p.T("Language"))
			assert.NotEmpty(t, p.N("%[1]s device", "%[1]s devices", i))
		})
	}

	wg.Wait()
}

func TestMessageMissingFromCatalogRendersEnglish(t *testing.T) {
	t.Parallel()

	// Throwaway catalog that lacks "Sign in", as after a msgid is removed or renamed.
	const po = `msgid ""
msgstr ""
"Language: it\n"
"Plural-Forms: nplurals=2; plural=(n != 1);\n"

msgid "Other"
msgstr "Altro"
`

	p := printer{tag: language.Italian, dir: dirLTR, loc: poLocale("it", po)}
	assert.Equal(t, "Sign in", p.T("Sign in"))
}

func TestNPluralizesItalianThroughCatalog(t *testing.T) {
	t.Parallel()

	// Throwaway catalog: the shipped it.po carries no plural entry, so the plural path is tested here.
	const po = `msgid ""
msgstr ""
"Language: it\n"
"Content-Type: text/plain; charset=UTF-8\n"
"Plural-Forms: nplurals=2; plural=(n != 1);\n"

msgid "%[1]s device"
msgid_plural "%[1]s devices"
msgstr[0] "%[1]s dispositivo"
msgstr[1] "%[1]s dispositivi"
`

	p := printer{tag: language.Italian, dir: dirLTR, loc: poLocale("it", po)}
	assert.Equal(t, "0 dispositivi", p.N("%[1]s device", "%[1]s devices", 0))
	assert.Equal(t, "1 dispositivo", p.N("%[1]s device", "%[1]s devices", 1))
	assert.Equal(t, "2 dispositivi", p.N("%[1]s device", "%[1]s devices", 2))
}
