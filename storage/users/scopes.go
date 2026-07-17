// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package users

import (
	"fmt"
	"sort"
	"strings"
)

// Scopes is a bitmask representation of RBAC access. Scopes are done in groups
// of four bits to denote Read, Update, Create, Delete. This 64-bit value gives
// room for 16 different resources.
type Scopes uint64

const (
	scopeR Scopes = 1 << 0
	scopeU Scopes = 1 << 1
	scopeC Scopes = 1 << 2
	scopeD Scopes = 1 << 3

	scopeShiftDevices Scopes = 0
	scopeShiftUpdates Scopes = 4
	scopeShiftUsers   Scopes = 8

	ScopeDevicesR  = scopeR << scopeShiftDevices
	ScopeDevicesRU = (scopeU | scopeR) << scopeShiftDevices
	ScopeDevicesC  = scopeC << scopeShiftDevices
	ScopeDevicesD  = scopeD << scopeShiftDevices

	ScopeUpdatesR  = scopeR << scopeShiftUpdates
	ScopeUpdatesRU = (scopeU | scopeR) << scopeShiftUpdates

	ScopeUsersR  = scopeR << scopeShiftUsers
	ScopeUsersRU = (scopeU | scopeR) << scopeShiftUsers
	ScopeUsersC  = scopeC << scopeShiftUsers
	ScopeUsersD  = scopeD << scopeShiftUsers
)

var maskToString = map[Scopes]string{
	ScopeDevicesR:  "devices:read",
	ScopeDevicesRU: "devices:read-update",
	ScopeDevicesC:  "devices:create",
	ScopeDevicesD:  "devices:delete",

	ScopeUpdatesR:  "updates:read",
	ScopeUpdatesRU: "updates:read-update",

	ScopeUsersR:  "users:read",
	ScopeUsersRU: "users:read-update",
	ScopeUsersC:  "users:create",
	ScopeUsersD:  "users:delete",
}

var stringToMask = map[string]Scopes{}
var allScopes []string
var maskInclusions = map[Scopes][]Scopes{}

func init() {
	for k, v := range maskToString {
		stringToMask[v] = k
		allScopes = append(allScopes, v)

		// keep up with supersets. eg. read-update includes read
		var sups []Scopes
		for super := range maskToString {
			if k&super == k && k != super {
				sups = append(sups, super)
			}
		}
		if len(sups) > 0 {
			maskInclusions[k] = sups
		}
	}
	sort.Strings(allScopes)
}

// ScopesAvailable returns a list of all available scopes as strings for display purposes.
func ScopesAvailable() []string {
	return allScopes
}

// ScopesFromString parses a comma-separated list of scopes into a Scopes bitmask.
func ScopesFromString(scopes string) (Scopes, error) {
	return ScopesFromSlice(strings.Split(scopes, ","))
}

// ScopesFromSlice parses a slice of scope strings into a Scopes bitmask.
func ScopesFromSlice(scopes []string) (Scopes, error) {
	var s Scopes
	for _, scope := range scopes {
		if v, ok := stringToMask[strings.TrimSpace(scope)]; ok {
			s |= v
		} else {
			return 0, fmt.Errorf("invalid scope: `%s`", scope)
		}
	}
	return s, nil
}

func (s Scopes) String() string {
	return strings.Join(s.ToSlice(), ",")
}

func (s Scopes) ToSlice() []string {
	var result []string
OUTER:
	for k, v := range maskToString {
		if s&k == k {
			if sups, ok := maskInclusions[k]; ok {
				// Check if this mask also contains one of the "super" masks of k.
				for _, kk := range sups {
					if s&kk == kk {
						continue OUTER
					}
				}
			}
			result = append(result, v)
		}
	}
	sort.Strings(result)
	return result
}

func (s Scopes) Has(scope Scopes) bool {
	return s&scope == scope
}
