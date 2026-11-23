// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
)

// scanGo returns every T and N call whose message arguments are string
// literals in the non-test Go files under dir. Other calls are ignored.
// The receiver type is not checked; phase 2 of the design adds that.
func scanGo(root, dir string) ([]Message, error) {
	var msgs []Message

	var errs []error

	fset := token.NewFileSet()

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		name, err := relPath(root, path)
		if err != nil {
			return err
		}

		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			errs = append(errs, err)
			return nil
		}

		at := func(p token.Pos) string {
			pos := fset.Position(p)
			return fmt.Sprintf("%s:%d:%d", name, pos.Line, pos.Column)
		}

		ast.Inspect(f, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := c.Fun.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != fnT && sel.Sel.Name != fnN) {
				return true
			}

			fn := sel.Sel.Name

			want := 1
			if fn == fnN {
				want = 2
			}

			if len(c.Args) < want {
				errs = append(errs, fmt.Errorf("%s: .%s: missing message string literal", at(c.Pos()), fn))
				return true
			}

			var args []string

			for _, a := range c.Args[:want] {
				lit, ok := a.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					errs = append(errs, fmt.Errorf("%s: .%s: message argument is not a string literal", at(a.Pos()), fn))
					return true
				}

				s, err := strconv.Unquote(lit.Value)
				if err != nil {
					errs = append(errs, fmt.Errorf("%s: .%s: %w", at(a.Pos()), fn, err))
					return true
				}

				args = append(args, s)
			}

			m := Message{ID: args[0], Refs: []Ref{{File: name, Line: fset.Position(sel.Sel.Pos()).Line}}}
			if fn == fnN {
				m.Plural = args[1]
			}

			msgs = append(msgs, m)

			return true
		})

		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return msgs, nil
}
