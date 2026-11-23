// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
)

const potHeader = `msgid ""
msgstr ""
"Plural-Forms: nplurals=2; plural=(n != 1);\n"
"MIME-Version: 1.0\n"
"Content-Type: text/plain; charset=UTF-8\n"
"Content-Transfer-Encoding: 8bit\n"
"Language: \n"
"X-Generator: i18nscan\n"
`

const (
	// xgettext wraps #: lines here even with --no-wrap.
	refWidth     = 79
	flagGoFormat = "go-format"
)

var escaper = strings.NewReplacer(
	`"`, `\"`, `\`, `\\`, "\n", `\n`, "\t", `\t`, "\r", `\r`,
	"\a", `\a`, "\b", `\b`, "\f", `\f`, "\v", `\v`,
)

// writePOT renders merged messages exactly as xgettext 1.0 would, with our header.
func writePOT(msgs []Message) ([]byte, error) {
	sorted := slices.Clone(msgs)
	slices.SortFunc(sorted, func(a, b Message) int {
		return cmp.Or(strings.Compare(a.ID, b.ID), strings.Compare(a.Ctx, b.Ctx))
	})

	var b bytes.Buffer

	b.WriteString(potHeader)

	var errs []error

	for _, m := range sorted {
		if err := writeEntry(&b, m); err != nil {
			errs = append(errs, err)
		}
	}

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

func writeEntry(b *bytes.Buffer, m Message) error {
	for _, r := range m.Refs {
		if strings.ContainsAny(r.File, " \t\n") {
			return fmt.Errorf("%s: path contains whitespace, which xgettext quotes and i18nscan does not", r)
		}
	}

	b.WriteByte('\n')

	for _, c := range m.Comments {
		if c == "" {
			b.WriteString("#.\n")

			continue
		}

		b.WriteString("#. " + c + "\n")
	}

	if len(m.Refs) > 0 {
		for _, l := range refLines(m.Refs) {
			b.WriteString(l + "\n")
		}
	}

	if isGoFormatPair(m.ID, m.Plural) {
		b.WriteString("#, " + flagGoFormat + "\n")
	}

	if m.Ctx != "" {
		field(b, "msgctxt", m.Ctx)
	}

	field(b, "msgid", m.ID)

	if m.Plural == "" {
		b.WriteString("msgstr \"\"\n")

		return nil
	}

	field(b, "msgid_plural", m.Plural)
	b.WriteString("msgstr[0] \"\"\nmsgstr[1] \"\"\n")

	return nil
}

// refLines packs references into #: lines, breaking only between references.
func refLines(rs []Ref) []string {
	var lines []string

	cur := "#:"

	for _, r := range rs {
		s := r.String()
		if cur != "#:" && len(cur)+1+len(s) > refWidth {
			lines = append(lines, cur)
			cur = "#:"
		}

		cur += " " + s
	}

	return append(lines, cur)
}

// field writes one keyword. A string with a newline before its last byte
// uses the "" continuation form, one quoted line per segment.
func field(b *bytes.Buffer, kw, s string) {
	segs := segments(s)
	if len(segs) <= 1 {
		b.WriteString(kw + ` "` + escaper.Replace(s) + "\"\n")

		return
	}

	b.WriteString(kw + " \"\"\n")

	for _, seg := range segs {
		b.WriteString(`"` + escaper.Replace(seg) + "\"\n")
	}
}

// segments splits s after every newline.
func segments(s string) []string {
	var out []string

	for {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			break
		}

		out = append(out, s[:i+1])
		s = s[i+1:]
	}

	if s != "" {
		out = append(out, s)
	}

	return out
}
