// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"text/template/parse"
)

const (
	fnT = "T"
	fnN = "N"
)

// scanText returns every .L.T and .L.N call in text. A call whose message
// arguments are not string literals is reported as an error.
// Dot rebinding inside range and with is not tracked, so .L.T there is still
// extracted.
func scanText(name, text string) ([]Message, error) {
	treeSet := map[string]*parse.Tree{}
	tree := parse.New(name)
	tree.Mode = parse.SkipFuncCheck

	if _, err := tree.Parse(text, "", "", treeSet); err != nil {
		return nil, err
	}

	s := &scanner{name: name, text: text}
	for _, n := range slices.Sorted(maps.Keys(treeSet)) {
		s.tree = treeSet[n]
		s.node(s.tree.Root)
	}

	return s.msgs, errors.Join(s.errs...)
}

type scanner struct {
	name string
	text string
	tree *parse.Tree
	msgs []Message
	errs []error
}

// line is the 1-based line of n in the template text.
func (s *scanner) line(n parse.Node) int {
	return 1 + strings.Count(s.text[:n.Position()], "\n")
}

func (s *scanner) node(n parse.Node) {
	switch n := n.(type) {
	case *parse.ListNode:
		if n == nil {
			return
		}

		for _, c := range n.Nodes {
			s.node(c)
		}
	case *parse.ActionNode:
		s.pipe(n.Pipe)
	case *parse.TemplateNode:
		s.pipe(n.Pipe)
	case *parse.IfNode:
		s.branch(&n.BranchNode)
	case *parse.RangeNode:
		s.branch(&n.BranchNode)
	case *parse.WithNode:
		s.branch(&n.BranchNode)
	}
}

func (s *scanner) branch(b *parse.BranchNode) {
	s.pipe(b.Pipe)
	s.node(b.List)
	s.node(b.ElseList)
}

func (s *scanner) pipe(p *parse.PipeNode) {
	if p == nil {
		return
	}

	for i, c := range p.Cmds {
		s.command(c, i == 0)
	}
}

// arg descends into parenthesized pipelines, where calls can also appear.
func (s *scanner) arg(n parse.Node) {
	switch n := n.(type) {
	case *parse.PipeNode:
		s.pipe(n)
	case *parse.ChainNode:
		s.arg(n.Node)
	}
}

// command records a .L.T or .L.N call. first is false when the command takes
// a piped value, which is an implicit message argument the scanner cannot see.
func (s *scanner) command(c *parse.CommandNode, first bool) {
	if len(c.Args) == 0 {
		return
	}

	fn, ok := lFunc(c.Args[0])
	if !ok {
		for _, a := range c.Args {
			s.arg(a)
		}

		return
	}

	if !first {
		s.fail(c, fn, "piped value is not a string literal")
		return
	}

	want := 1
	if fn == fnN {
		want = 2
	}

	msgs := c.Args[1:]
	if len(msgs) < want {
		s.fail(c, fn, "missing message string literal")
		return
	}

	var args []string

	for _, a := range msgs[:want] {
		lit, ok := a.(*parse.StringNode)
		if !ok {
			s.fail(a, fn, "message argument is not a string literal")
			return
		}

		args = append(args, lit.Text)
	}

	m := Message{ID: args[0], Refs: []Ref{{File: s.name, Line: s.line(c.Args[0])}}}
	if fn == fnN {
		m.Plural = args[1]
	}

	s.msgs = append(s.msgs, m)
}

func (s *scanner) fail(n parse.Node, fn, reason string) {
	loc, _ := s.tree.ErrorContext(n)
	s.errs = append(s.errs, fmt.Errorf("%s: .L.%s: %s", loc, fn, reason))
}

// lFunc matches the .L.T and .L.N chains. Other chains such as .L.Tag are
// different identifiers and do not match.
func lFunc(n parse.Node) (string, bool) {
	var ident []string

	switch n := n.(type) {
	case *parse.FieldNode:
		ident = n.Ident
	case *parse.VariableNode:
		ident = n.Ident
	default:
		return "", false
	}

	switch {
	case slices.Equal(ident, []string{"L", "T"}), slices.Equal(ident, []string{"$", "L", "T"}):
		return fnT, true
	case slices.Equal(ident, []string{"L", "N"}), slices.Equal(ident, []string{"$", "L", "N"}):
		return fnN, true
	}

	return "", false
}
