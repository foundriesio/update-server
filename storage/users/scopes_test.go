// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package users

import (
	"slices"
	"testing"
)

func TestScopesFromString(t *testing.T) {
	tests := []struct {
		name    string // description of this test case
		scopes  string
		want    Scopes
		has     []Scopes
		wantErr bool
	}{
		{
			name:    "Invalid resource",
			scopes:  "device:read,users:read-update",
			wantErr: true,
		},
		{
			name:    "Invalid scope",
			scopes:  "devices:read,users:read-updat",
			wantErr: true,
		},
		{
			name:    "Nonexistent scope",
			scopes:  "devices:read,updates:create",
			wantErr: true,
		},
		{
			name:   "Updates delete scope",
			scopes: "updates:delete",
			want:   ScopeUpdatesD,
			has:    []Scopes{ScopeUpdatesD},
		},
		{
			name:   "Handle white space",
			scopes: "devices:read, users:read-update",
			want:   ScopeDevicesR | ScopeUsersRU,
			has:    []Scopes{ScopeDevicesR, ScopeUsersR},
		},
		{
			name:   "Normalize supersets",
			scopes: "devices:read, devices:read-update,updates:read",
			want:   ScopeDevicesRU | ScopeUpdatesR,
			has:    []Scopes{ScopeDevicesR, ScopeDevicesRU, ScopeUpdatesR},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := ScopesFromString(tt.scopes)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("ScopesFromString() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("ScopesFromString() succeeded unexpectedly")
			}

			if got != tt.want {
				t.Errorf("ScopesFromString() = %v, want %v", got, tt.want)
			}

			for _, h := range tt.has {
				if !got.Has(h) {
					t.Errorf("ScopesFromString().Has(%v) = false, want true", h)
				}
			}
			if got.Has(ScopeDevicesD) {
				t.Errorf("ScopesFromString().Has(devices:delete) = true, want false")
			}
		})
	}
}

func TestScopes_ToSlice(t *testing.T) {
	tests := []struct {
		name   string // description of this test case
		scopes Scopes
		want   []string
	}{
		{
			name:   "devices:read and users:read-update",
			scopes: ScopeDevicesR | ScopeUsersRU,
			want:   []string{"devices:read", "users:read-update"},
		},
		{
			name:   "devices:read, users:read, users:delete",
			scopes: ScopeDevicesR | ScopeUsersR | ScopeUsersD,
			want:   []string{"devices:read", "users:delete", "users:read"},
		},
		{
			name:   "users:read-update, users:create, updates:read-update",
			scopes: ScopeUsersRU | ScopeUsersC | ScopeUpdatesRU,
			want:   []string{"updates:read-update", "users:create", "users:read-update"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asSlice := tt.scopes.ToSlice()
			if !slices.Equal(asSlice, tt.want) {
				t.Fatalf("ToSlice() = %v, want %v", asSlice, tt.want)
			}

			got, err := ScopesFromSlice(asSlice)
			if err != nil {
				t.Fatalf("ScopesFromSlice() failed: %v", err)
			}
			if got != tt.scopes {
				t.Errorf("ScopesFromSlice() = %v, want %v", got, tt.scopes)
			}
		})
	}
}
