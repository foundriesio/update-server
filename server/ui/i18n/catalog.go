// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package i18n

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/leonelquinteros/gotext"
	"golang.org/x/text/language"
)

// Embedded from locale/ only; PO sources stay outside the binary.
// all: keeps locale/.gitkeep, so the directory exists before any catalog ships.
//
//go:embed all:locale
var catalogFS embed.FS

const (
	catalogRoot   = "locale"
	catalogDomain = "ui"

	// Present in every shipped catalog; a catalog that lacks it is treated as absent.
	probeKey = "Save changes"
)

var printers map[string]Printer

func init() {
	var err error

	printers, err = loadPrinters(catalogFS)

	// Missing catalog falls back to English; startup must not stop.
	if err != nil {
		slog.Error("i18n catalogs incomplete", "error", err)
	}
}

func newPrinter(tag language.Tag, fsys fs.FS) printer {
	p := printer{tag: tag, dir: dirLTR}
	for _, l := range Languages() {
		if l.Tag == tag.String() {
			p.dir = l.Dir
		}
	}

	if tag == language.English {
		return p
	}

	loc := gotext.NewLocaleFSWithPath(tag.String(), fsys, catalogRoot)
	loc.AddDomain(catalogDomain)
	p.loc = loc

	return p
}

// AddDomain fails silently when no catalog exists, so a missing file would serve English unnoticed.
func loadPrinters(fsys fs.FS) (map[string]Printer, error) {
	out := make(map[string]Printer, len(supported))

	var errs []error

	for _, tag := range supported {
		p := newPrinter(tag, fsys)
		out[tag.String()] = p

		if tag == language.English {
			continue
		}

		if p.T(probeKey) == probeKey {
			errs = append(errs, fmt.Errorf("i18n catalog %s: %q not translated", tag.String(), probeKey))
		}
	}

	return out, errors.Join(errs...)
}
