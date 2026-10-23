// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package i18n

const dirLTR = "ltr"

// Language is one settings-selector entry; Name is the language's own name.
type Language struct{ Tag, Name, Dir string }

// Keys must cover every LINGUAS tag; TestLanguagesHasEntryForEverySupportedTag enforces it.
var languageInfo = map[string]Language{
	"en": {Tag: "en", Name: "English", Dir: dirLTR},
	"it": {Tag: "it", Name: "Italiano", Dir: dirLTR},
}

// Languages returns the selector entries in LINGUAS order.
func Languages() []Language {
	out := make([]Language, 0, len(supported))
	for _, tag := range supported {
		out = append(out, languageInfo[tag.String()])
	}

	return out
}
