// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package templates

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"

	"github.com/foundriesio/update-server/server/ui/i18n"
	"github.com/foundriesio/update-server/server/ui/pagectx"
)

// English msgids that must not survive in Italian output.
var englishLeakMsgids = []string{
	"Fleet updates, under control.",
	"Username",
	"Switch to dark mode",
	"Toggle navigation",
	"Account access",
}

type localeTestPage struct {
	pagectx.Base
	Reason      string
	LoginTip    string
	Name        string
	ProviderKey string
	Next        string
}

func renderLocalePage(t *testing.T, name string, tag language.Tag) string {
	t.Helper()

	data := localeTestPage{
		Base: pagectx.Base{
			L:         i18n.For(tag),
			Title:     "Login",
			BrandName: "Update Server",
			Version:   "test",
		},
		Name:        "GitHub",
		ProviderKey: "github",
	}

	// Execute a clone: running the shared set would break TestJSONEscapesScriptEndTag's Clone.
	tmpl, err := Templates.Clone()
	require.NoError(t, err, "Clone()")

	var buf bytes.Buffer
	require.NoError(t, tmpl.ExecuteTemplate(&buf, name, data), "ExecuteTemplate(%s, %s)", name, tag)

	return buf.String()
}

func TestLocalePagesRenderTranslatedMarkup(t *testing.T) {
	tests := []struct {
		name string
		want string
		tag  language.Tag
		// Italian strings must appear and English ones must not, or the page is still untranslated.
		present string
		absent  string
	}{
		{name: "header", tag: language.Italian, want: `<html lang="it" dir="ltr">`, present: "Vai al contenuto principale", absent: "Skip to main content"},
		{name: "header", tag: language.English, want: `<html lang="en" dir="ltr">`, present: "Skip to main content"},
		{name: "local-login.html", tag: language.Italian, want: `<html lang="it" dir="ltr">`, present: "Accedi", absent: "Sign in"},
		{name: "local-login.html", tag: language.English, want: `<html lang="en" dir="ltr">`, present: "Sign in"},
		{name: "oauth2-login.html", tag: language.Italian, want: `<html lang="it" dir="ltr">`, present: "Accedi", absent: "Sign in"},
		{name: "oauth2-login.html", tag: language.English, want: `<html lang="en" dir="ltr">`, present: "Sign in"},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/"+tt.tag.String(), func(t *testing.T) {
			got := renderLocalePage(t, tt.name, tt.tag)

			assert.Contains(t, got, tt.want)
			assert.Contains(t, got, tt.present)
			if tt.absent != "" {
				assert.NotContains(t, got, tt.absent, "English text in %s output", tt.tag)
			}
			assert.NotContains(t, got, "%!", "formatting error in output")
			assert.NotContains(t, got, "%[1]", "unformatted placeholder in output")

			if tt.tag != language.Italian {
				return
			}
			for _, msgid := range englishLeakMsgids {
				// Skip msgids whose Italian equals the English, since they cannot be detected.
				if i18n.For(language.Italian).T(msgid) == msgid {
					continue
				}
				assert.NotContains(t, got, msgid)
			}
		})
	}
}

func TestLocalePlaceholdersRenderProviderAndProduct(t *testing.T) {
	oauth := renderLocalePage(t, "oauth2-login.html", language.Italian)
	assert.Contains(t, oauth, "Continua con GitHub")
	assert.Contains(t, oauth, "Accesso tramite account GitHub")

	local := renderLocalePage(t, "local-login.html", language.Italian)
	assert.Contains(t, local, "Usa le credenziali di Update Server.")
}
