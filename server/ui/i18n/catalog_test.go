// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package i18n

import (
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/leonelquinteros/gotext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"
)

func TestLoadPrintersFailsWithoutCatalog(t *testing.T) {
	t.Parallel()

	_, err := loadPrinters(fstest.MapFS{})
	assert.ErrorContains(t, err, "i18n catalog it")
}

func TestLoadPrintersPassesFixtureCatalog(t *testing.T) {
	t.Parallel()

	_, err := loadPrinters(os.DirFS("testdata"))
	assert.NoError(t, err)
}

func TestEmbeddedCatalogHasNoPo(t *testing.T) {
	t.Parallel()

	err := fs.WalkDir(catalogFS, ".", func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// gotext reads .po before .mo, so a shipped .po would shadow the compiled catalog.
		if strings.HasSuffix(path, ".po") {
			t.Errorf("embedded catalog contains PO source: %s", path)
		}

		return nil
	})
	require.NoError(t, err)
}

func TestFixtureCatalogMatchesPo(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("testdata/it.po")
	require.NoError(t, err)

	// gotext does not expose the fuzzy flag, so fail on it rather than skip the entry.
	require.NotContains(t, string(data), "#, fuzzy")

	po := gotext.NewPo()
	po.Parse(data)

	p := For(language.Italian)
	checked := 0

	for _, tr := range po.GetDomain().GetTranslations() {
		// The header entry has an empty msgid.
		if tr.ID == "" || tr.Get() == "" {
			continue
		}

		got := p.T(tr.ID)
		if tr.PluralID != "" {
			got = p.N(tr.ID, tr.PluralID, 1)
		}

		checked++
		assert.NotEqual(t, tr.ID, got, "msgid %q missing from fixture ui.mo", tr.ID)
	}

	require.NotZero(t, checked, "no translated entries parsed from testdata/it.po")
}

// Throwaway catalogs, not shipped: they exercise gotext's plural selection
// for languages whose formulas are not in the embedded catalog.
const plPO = `msgid ""
msgstr ""
"Language: pl\n"
"Content-Type: text/plain; charset=UTF-8\n"
"Plural-Forms: nplurals=3; plural=(n==1 ? 0 : n%10>=2 && n%10<=4 && (n%100<10 || n%100>=20) ? 1 : 2);\n"

msgid "%[1]s device"
msgid_plural "%[1]s devices"
msgstr[0] "form0 %[1]s"
msgstr[1] "form1 %[1]s"
msgstr[2] "form2 %[1]s"
`

const arPO = `msgid ""
msgstr ""
"Language: ar\n"
"Content-Type: text/plain; charset=UTF-8\n"
"Plural-Forms: nplurals=6; plural=(n==0 ? 0 : n==1 ? 1 : n==2 ? 2 : n%100>=3 && n%100<=10 ? 3 : n%100>=11 ? 4 : 5);\n"

msgid "%[1]s device"
msgid_plural "%[1]s devices"
msgstr[0] "form0 %[1]s"
msgstr[1] "form1 %[1]s"
msgstr[2] "form2 %[1]s"
msgstr[3] "form3 %[1]s"
msgstr[4] "form4 %[1]s"
msgstr[5] "form5 %[1]s"
`

func poLocale(tag, po string) *gotext.Locale {
	fsys := fstest.MapFS{"locale/" + tag + "/LC_MESSAGES/ui.po": {Data: []byte(po)}}
	loc := gotext.NewLocaleFSWithPath(tag, fsys, catalogRoot)
	loc.AddDomain(catalogDomain)

	return loc
}

func TestPluralForms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		po    string
		tag   string
		forms map[int]int
	}{
		{name: "polish", po: plPO, tag: "pl", forms: map[int]int{1: 0, 2: 1, 5: 2, 22: 1}},
		{name: "arabic", po: arPO, tag: "ar", forms: map[int]int{0: 0, 1: 1, 2: 2, 3: 3, 11: 4, 100: 5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			loc := poLocale(tt.tag, tt.po)
			for n, form := range tt.forms {
				got := loc.GetN("%[1]s device", "%[1]s devices", n, strconv.Itoa(n))
				assert.Equal(t, fmt.Sprintf("form%d %d", form, n), got, "n=%d", n)
			}
		})
	}
}
