// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package i18n

import (
	"time"

	"github.com/leonelquinteros/gotext"
	"golang.org/x/text/language"
)

// Printer renders one locale's messages, dates and numbers. Values are
// immutable after For returns and safe for concurrent use.
type Printer interface {
	T(key string, args ...any) string
	N(singular, plural string, n int, args ...any) string
	Tag() string
	Dir() string
	Date(t time.Time) string
	DateTime(t time.Time) string
	Number(n int) string
}

// For returns the printer for tag, or the English printer when tag is unsupported.
// tag must be canonical, as returned by Resolve; other tags get English.
func For(tag language.Tag) Printer {
	if p, ok := printers[tag.String()]; ok {
		return p
	}

	return printers[language.English.String()]
}

// printer has a nil loc for English, whose keys are the English text.
type printer struct {
	tag language.Tag
	dir string
	loc *gotext.Locale
}

func (p printer) T(key string, args ...any) string {
	if p.loc != nil {
		return p.loc.Get(key, args...)
	}

	return gotext.FormatString(key, args...)
}

// Every form must contain %[1]s, else fmt appends %!(EXTRA ...); the count is arg 1.
func (p printer) N(singular, plural string, n int, args ...any) string {
	vars := make([]any, 0, 1+len(args))
	vars = append(vars, p.Number(n))
	vars = append(vars, args...)

	if p.loc != nil {
		return p.loc.GetN(singular, plural, n, vars...)
	}

	if n == 1 {
		return p.T(singular, vars...)
	}

	return p.T(plural, vars...)
}

func (p printer) Tag() string {
	return p.tag.String()
}

func (p printer) Dir() string {
	return p.dir
}

func (p printer) Date(t time.Time) string {
	return formatDate(p.tag, t)
}

func (p printer) DateTime(t time.Time) string {
	return formatDateTime(p.tag, t)
}

func (p printer) Number(n int) string {
	return formatNumber(p.tag, n)
}
