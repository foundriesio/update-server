// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package i18n

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/text/language"
)

func TestDateEveryMonth(t *testing.T) {
	t.Parallel()

	tests := map[language.Tag][12]string{
		language.English: {
			"Jan 23, 2026", "Feb 23, 2026", "Mar 23, 2026", "Apr 23, 2026",
			"May 23, 2026", "Jun 23, 2026", "Jul 23, 2026", "Aug 23, 2026",
			"Sep 23, 2026", "Oct 23, 2026", "Nov 23, 2026", "Dec 23, 2026",
		},
		language.Italian: {
			"23 gen 2026", "23 feb 2026", "23 mar 2026", "23 apr 2026",
			"23 mag 2026", "23 giu 2026", "23 lug 2026", "23 ago 2026",
			"23 set 2026", "23 ott 2026", "23 nov 2026", "23 dic 2026",
		},
	}

	for tag, months := range tests {
		for i, want := range months {
			tm := time.Date(2026, time.Month(i+1), 23, 15, 45, 0, 0, time.UTC)
			assert.Equal(t, want, For(tag).Date(tm), "tag %s month %d", tag, i+1)
		}
	}
}

func TestDateSingleDigitDayNotPadded(t *testing.T) {
	t.Parallel()

	tm := time.Date(2026, time.October, 5, 15, 45, 0, 0, time.UTC)

	assert.Equal(t, "Oct 5, 2026", For(language.English).Date(tm))
	assert.Equal(t, "5 ott 2026", For(language.Italian).Date(tm))
}

func TestDateTimeClockPerLocale(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tag  language.Tag
		hour int
		want string
	}{
		{"en morning", language.English, 9, "Oct 23, 2026, 9:05 AM"},
		{"en afternoon", language.English, 15, "Oct 23, 2026, 3:05 PM"},
		{"en midnight", language.English, 0, "Oct 23, 2026, 12:05 AM"},
		{"en noon", language.English, 12, "Oct 23, 2026, 12:05 PM"},
		{"it midnight", language.Italian, 0, "23 ott 2026, 00:05"},
		{"it noon", language.Italian, 12, "23 ott 2026, 12:05"},
		{"it afternoon", language.Italian, 15, "23 ott 2026, 15:05"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tm := time.Date(2026, time.October, 23, tt.hour, 5, 0, 0, time.UTC)
			assert.Equal(t, tt.want, For(tt.tag).DateTime(tm))
		})
	}
}

func TestDateTimeKeepsTimeLocation(t *testing.T) {
	t.Parallel()

	// 00:30 at UTC+5 must not roll back to the previous day in UTC.
	zone := time.FixedZone("UTC+5", 5*60*60)
	tm := time.Date(2026, time.October, 23, 0, 30, 0, 0, zone)

	assert.Equal(t, "Oct 23, 2026, 12:30 AM", For(language.English).DateTime(tm))
}

func TestNumberPerLocale(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tag  language.Tag
		n    int
		want string
	}{
		{"en zero", language.English, 0, "0"},
		{"en 999", language.English, 999, "999"},
		{"en 1000", language.English, 1000, "1,000"},
		{"en 1234", language.English, 1234, "1,234"},
		{"en 1234567", language.English, 1234567, "1,234,567"},
		{"en negative", language.English, -1234567, "-1,234,567"},
		{"it zero", language.Italian, 0, "0"},
		{"it 999", language.Italian, 999, "999"},
		{"it 1000", language.Italian, 1000, "1.000"},
		{"it 1234", language.Italian, 1234, "1.234"},
		{"it 1234567", language.Italian, 1234567, "1.234.567"},
		{"it negative", language.Italian, -1234567, "-1.234.567"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, For(tt.tag).Number(tt.n))
		})
	}
}

func TestUnknownTagFormatsAsEnglish(t *testing.T) {
	t.Parallel()

	// French is not in the table; a French printer must not use French separators.
	p := newPrinter(language.French, catalogFS)
	tm := time.Date(2026, time.October, 23, 15, 45, 0, 0, time.UTC)

	assert.Equal(t, "Oct 23, 2026", p.Date(tm))
	assert.Equal(t, "Oct 23, 2026, 3:45 PM", p.DateTime(tm))
	assert.Equal(t, "1,234,567", p.Number(1234567))
}

func TestFormatConcurrentUse(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup

	tm := time.Date(2026, time.October, 23, 15, 45, 0, 0, time.UTC)

	for i := range 50 {
		wg.Go(func() {
			tag, wantDate, wantDateTime, wantNum := language.English, "Oct 23, 2026", "Oct 23, 2026, 3:45 PM", "1,234,567"
			if i%2 == 1 {
				tag, wantDate, wantDateTime, wantNum = language.Italian, "23 ott 2026", "23 ott 2026, 15:45", "1.234.567"
			}

			p := For(tag)
			assert.Equal(t, wantDate, p.Date(tm))
			assert.Equal(t, wantDateTime, p.DateTime(tm))
			assert.Equal(t, wantNum, p.Number(1234567))
		})
	}

	wg.Wait()
}
