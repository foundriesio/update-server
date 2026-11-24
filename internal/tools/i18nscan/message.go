// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strconv"
)

// Ref is one source position of a message.
type Ref struct {
	File string // slash-separated, relative to the scan root
	Line int
}

func (r Ref) String() string {
	return r.File + ":" + strconv.Itoa(r.Line)
}

// Message is one POT entry. Ctx and Plural are empty when unused.
type Message struct {
	Ctx      string
	ID       string
	Plural   string
	Comments []string
	Refs     []Ref
}

func cmpRef(a, b Ref) int {
	return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line))
}

func (m Message) where() string {
	if len(m.Refs) == 0 {
		return "unknown position"
	}

	return m.Refs[0].String()
}

// merge collapses messages with the same Ctx and ID, keeping first-seen order.
// Two sites that disagree on the plural are an error, never a silent drop.
func merge(in []Message) ([]Message, error) {
	type key struct{ ctx, id string }

	index := map[key]int{}

	var out []Message

	var errs []error

	for _, m := range in {
		k := key{m.Ctx, m.ID}

		i, seen := index[k]
		if !seen {
			index[k] = len(out)
			m.Refs = slices.Clone(m.Refs)
			m.Comments = slices.Clone(m.Comments)
			out = append(out, m)

			continue
		}

		cur := &out[i]
		if cur.Plural != m.Plural {
			errs = append(errs, fmt.Errorf("%s: %q: msgid_plural %q conflicts with %q at %s",
				m.where(), m.ID, m.Plural, cur.Plural, cur.where()))

			continue
		}

		cur.Refs = append(cur.Refs, m.Refs...)

		for _, c := range m.Comments {
			if !slices.Contains(cur.Comments, c) {
				cur.Comments = append(cur.Comments, c)
			}
		}
	}

	for i := range out {
		slices.SortFunc(out[i].Refs, cmpRef)
		out[i].Refs = slices.Compact(out[i].Refs)
	}

	return out, errors.Join(errs...)
}
