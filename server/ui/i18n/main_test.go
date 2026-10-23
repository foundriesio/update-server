// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package i18n

import (
	"os"
	"testing"

	"golang.org/x/text/language"
)

// LINGUAS ships only en until the first translation lands, so tests
// add a second language to exercise matching and the selector list.
func TestMain(m *testing.M) {
	supported = []language.Tag{language.English, language.Italian}
	matcher = language.NewMatcher(supported)

	// The shipped locale/ holds no catalog yet, so printers come from the fixture.
	var err error

	if printers, err = loadPrinters(os.DirFS("testdata")); err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}
