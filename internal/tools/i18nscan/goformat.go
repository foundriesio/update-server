// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"fmt"
	"strconv"
	"strings"
)

// goVerbs are the verbs GNU format-go.c accepts; %p and %w are not among them.
const goVerbs = "bcdeEfFgGoOqstTUvxX"

// Argument kinds, as bit sets. Within one string the kinds used for an
// argument are intersected (empty means invalid); across msgid and
// msgid_plural the resulting sets must be equal. The bits reproduce what
// xgettext 1.0 does: O and a width or precision (*) accept only kindInt, d adds kindDec, b adds kindFloat
// and x adds kindStr, so each verb is a subset of the next.
const (
	kindInt uint8 = 1 << iota
	kindDec
	kindFloat
	kindStr
	kindRune
	kindBool
	kindType
)

// directive is one argument use. verb is '*' for a width or precision.
type directive struct {
	arg  int
	verb byte
}

// isGoFormat reports whether xgettext marks s as go-format.
func isGoFormat(s string) bool {
	_, ok := parseFormat(s)

	return ok
}

// isGoFormatPair reports whether xgettext marks a plural entry as go-format:
// at least one side is a format and neither side is an invalid one.
func isGoFormatPair(id, plural string) bool {
	_, aok := parseFormat(id)
	_, bok := parseFormat(plural)

	if badFormat(id, aok) || badFormat(plural, bok) {
		return false
	}

	return aok || bok
}

// parseFormat decodes Go format directives the way GNU format-go.c does. ok is
// false when s has no directive or any directive is invalid.
func parseFormat(s string) ([]directive, bool) {
	var dirs []directive

	arg := 1
	seen := false

	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			continue
		}

		seen = true
		i++

		for i < len(s) && strings.IndexByte("+-#0", s[i]) >= 0 {
			i++
		}

		var ok bool

		if i, arg, ok = argIndex(s, i, arg); !ok {
			return nil, false
		}

		if i < len(s) && s[i] == '*' {
			dirs = append(dirs, directive{arg, '*'})
			arg++
			i++
		} else {
			i = skipDigits(s, i)
		}

		if i < len(s) && s[i] == '.' {
			i++

			if i, arg, ok = argIndex(s, i, arg); !ok {
				return nil, false
			}

			if i < len(s) && s[i] == '*' {
				dirs = append(dirs, directive{arg, '*'})
				arg++
				i++
			} else {
				i = skipDigits(s, i)
			}
		}

		if i, arg, ok = argIndex(s, i, arg); !ok {
			return nil, false
		}

		if i >= len(s) {
			return nil, false
		}

		verb := s[i]
		if verb == '%' {
			continue
		}

		if strings.IndexByte(goVerbs, verb) < 0 {
			return nil, false
		}

		dirs = append(dirs, directive{arg, verb})
		arg++
	}

	if _, ok := argKinds(dirs); !ok {
		return nil, false
	}

	return dirs, seen
}

func skipDigits(s string, i int) int {
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}

	return i
}

// maxArgIndex is the largest [n] xgettext 1.0 accepts (found by bisection).
const maxArgIndex = 1000000

// argIndex consumes an optional [n] at s[i:], returning the new position and
// the argument number it selects.
func argIndex(s string, i, arg int) (int, int, bool) {
	if i >= len(s) || s[i] != '[' {
		return i, arg, true
	}

	end := strings.IndexByte(s[i:], ']')
	if end < 0 {
		return i, arg, false
	}

	digits := s[i+1 : i+end]
	if digits == "" || skipDigits(digits, 0) != len(digits) {
		return i, arg, false
	}

	n, err := strconv.Atoi(digits)
	if err != nil || n < 1 || n > maxArgIndex {
		return i, arg, false
	}

	return i + end + 1, n, true
}

// verbKinds is the set of kinds a verb accepts; verbs with equal sets are
// interchangeable in a msgid/msgid_plural pair (e.g. d and o, x and X).
func verbKinds(v byte) uint8 {
	switch v {
	case 'O', '*':
		return kindInt
	case 'd', 'o':
		return kindInt | kindDec
	case 'b':
		return kindInt | kindDec | kindFloat
	case 'e', 'E', 'f', 'F', 'g', 'G':
		return kindFloat
	case 'x', 'X':
		return kindInt | kindDec | kindFloat | kindStr
	case 's':
		return kindStr
	case 'q':
		return kindRune | kindStr
	case 'c', 'U':
		return kindRune
	case 't':
		return kindBool
	case 'T':
		return kindType
	case 'v':
		return kindInt | kindDec | kindFloat | kindStr | kindRune | kindBool
	}

	return 0
}

// argKinds intersects the kinds of every use of each argument. ok is false
// when an argument is used in incompatible ways, which makes xgettext treat
// the string as not a format.
func argKinds(dirs []directive) (map[int]uint8, bool) {
	kinds := map[int]uint8{}

	for _, d := range dirs {
		k := verbKinds(d.verb)
		if prev, found := kinds[d.arg]; found {
			k &= prev
		}

		if k == 0 {
			return nil, false
		}

		kinds[d.arg] = k
	}

	return kinds, true
}

// badFormat reports a string that has a '%' but is not a valid format.
func badFormat(s string, ok bool) bool {
	return !ok && strings.Contains(s, "%")
}

// checkPlural mirrors xgettext's refusal of a msgid_plural whose arguments
// differ from msgid's. The pair is not checked when either side is an
// invalid format; a side without any directive uses no arguments.
func checkPlural(id, plural string) error {
	a, aok := parseFormat(id)
	b, bok := parseFormat(plural)

	if badFormat(id, aok) || badFormat(plural, bok) {
		return nil
	}

	ak, _ := argKinds(a)
	bk, _ := argKinds(b)

	for _, d := range a {
		k, found := bk[d.arg]
		if !found {
			return fmt.Errorf("a format specification for argument %d doesn't exist in 'msgid_plural'", d.arg)
		}

		if k != ak[d.arg] {
			return fmt.Errorf("format specifications in 'msgid_plural' and 'msgid' for argument %d are not the same", d.arg)
		}
	}

	for _, d := range b {
		if _, found := ak[d.arg]; !found {
			return fmt.Errorf("a format specification for argument %d doesn't exist in 'msgid'", d.arg)
		}
	}

	return nil
}
