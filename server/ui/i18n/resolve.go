// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package i18n

import "golang.org/x/text/language"

var matcher = language.NewMatcher(supported)

const maxAcceptLanguage = 1024

// Valid reports whether tag is exactly a supported canonical tag.
func Valid(tag string) (language.Tag, bool) {
	for _, t := range supported {
		if t.String() == tag {
			return t, true
		}
	}

	return language.Und, false
}

// Resolve picks a stored locale if it is supported, else the best Accept-Language match, else English.
func Resolve(stored, acceptLanguage string) language.Tag {
	if tag, ok := Valid(stored); ok {
		return tag
	}

	// Bounds parse and match work on a client-controlled header.
	if len(acceptLanguage) > maxAcceptLanguage {
		return language.English
	}

	prefs, _, err := language.ParseAcceptLanguage(acceptLanguage)
	if err != nil {
		return language.English
	}

	// Index, not the matched tag: the matched tag carries request extensions such as -u-rg-chzzzz.
	_, idx, conf := matcher.Match(prefs...)
	if conf == language.No {
		return language.English
	}

	return supported[idx]
}
