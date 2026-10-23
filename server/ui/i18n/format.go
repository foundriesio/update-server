// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package i18n

import (
	"fmt"
	"time"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// Hand-written; to be replaced by generated CLDR data.
type dateTable struct {
	months [12]string
	// Arguments: day, month name, year.
	date string
	// Go time layout for the time of day.
	clock string
}

var dateTables = map[string]dateTable{
	"en": {
		months: [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
		date:   "%[2]s %[1]d, %[3]d",
		clock:  "3:04 PM", // The en clock puts an ASCII space before AM/PM where CLDR uses U+202F.
	},
	"it": {
		months: [12]string{"gen", "feb", "mar", "apr", "mag", "giu", "lug", "ago", "set", "ott", "nov", "dic"},
		date:   "%[1]d %[2]s %[3]d",
		clock:  "15:04",
	},
}

// tableTag returns tag when a date table covers it, else English, matching For's fallback.
func tableTag(tag language.Tag) language.Tag {
	if _, ok := dateTables[tag.String()]; ok {
		return tag
	}

	return language.English
}

// Each value formats in t's own location, so no conversion happens before the fields are read.
func formatDate(tag language.Tag, t time.Time) string {
	tbl := dateTables[tableTag(tag).String()]

	return fmt.Sprintf(tbl.date, t.Day(), tbl.months[t.Month()-1], t.Year())
}

func formatDateTime(tag language.Tag, t time.Time) string {
	tbl := dateTables[tableTag(tag).String()]

	return formatDate(tag, t) + ", " + t.Format(tbl.clock)
}

// message.Printer is not documented as safe for concurrent use, so each call builds its own.
func formatNumber(tag language.Tag, n int) string {
	return message.NewPrinter(tableTag(tag)).Sprintf("%d", n)
}
