// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeRefsSortedAndDeduped(t *testing.T) {
	in := []Message{
		{ID: "d", Refs: []Ref{{"b.html", 3}}},
		{ID: "d", Refs: []Ref{{"a.html", 9}}},
		{ID: "d", Refs: []Ref{{"a.html", 9}, {"a.html", 10}}},
	}

	got, err := merge(in)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, []Ref{{"a.html", 9}, {"a.html", 10}, {"b.html", 3}}, got[0].Refs)
}

func TestMergeKeepsFirstSeenOrder(t *testing.T) {
	got, err := merge([]Message{{ID: "b"}, {ID: "a"}, {ID: "b"}})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "b", got[0].ID)
	assert.Equal(t, "a", got[1].ID)
}

func TestMergeContextSeparates(t *testing.T) {
	got, err := merge([]Message{{ID: "d"}, {Ctx: "x", ID: "d"}})
	require.NoError(t, err)
	assert.Len(t, got, 2)
}

func TestMergeUnionsComments(t *testing.T) {
	got, err := merge([]Message{
		{ID: "d", Comments: []string{"one", "two"}},
		{ID: "d", Comments: []string{"two", "three"}},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"one", "two", "three"}, got[0].Comments)
}

func TestMergePluralConflict(t *testing.T) {
	tests := map[string][2]Message{
		"singular then plural": {
			{ID: "x", Refs: []Ref{{"a.go", 1}}},
			{ID: "x", Plural: "xs", Refs: []Ref{{"b.go", 2}}},
		},
		"different plurals": {
			{ID: "x", Plural: "xs", Refs: []Ref{{"a.go", 1}}},
			{ID: "x", Plural: "xes", Refs: []Ref{{"b.go", 2}}},
		},
	}
	for name, pair := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := merge(pair[:])
			require.Error(t, err)
			assert.ErrorContains(t, err, "a.go:1")
			assert.ErrorContains(t, err, "b.go:2")
		})
	}
}

func TestMergeDoesNotModifyInput(t *testing.T) {
	in := []Message{{ID: "d", Refs: []Ref{{"b.html", 3}}}, {ID: "d", Refs: []Ref{{"a.html", 1}}}}

	_, err := merge(in)
	require.NoError(t, err)
	assert.Equal(t, []Ref{{"b.html", 3}}, in[0].Refs)
}
