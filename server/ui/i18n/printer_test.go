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

func TestItalianTranslatesSaveChanges(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Salva le modifiche", For(language.Italian).T("Save changes"))
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

	assert.Equal(t, "Delete device dev-1?", For(language.English).T("Delete device %s?", "dev-1"))
	assert.Equal(t, "Eliminare il dispositivo dev-1?", For(language.Italian).T("Delete device %s?", "dev-1"))
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
		{language.Italian, 0, "0 dispositivi"},
		{language.Italian, 1, "1 dispositivo"},
		{language.Italian, 2, "2 dispositivi"},
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
	assert.Equal(t, "Save changes", p.T("Save changes"))
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
			p, want := For(language.English), "Save changes"
			if i%2 == 1 {
				p, want = For(language.Italian), "Salva le modifiche"
			}

			assert.Equal(t, want, p.T("Save changes"))
			assert.NotEmpty(t, p.N("%[1]s device", "%[1]s devices", i))
		})
	}

	wg.Wait()
}
