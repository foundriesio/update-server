// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package auth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoginRedirectTarget(t *testing.T) {
	cases := map[string]string{
		"":                                   "/",
		"/":                                  "/",
		"/auth/activate?user_code=ABCD-1234": "/auth/activate?user_code=ABCD-1234",
		"/devices/x?tab=updates":             "/devices/x?tab=updates",
		"//evil.example/":                    "/",
		"/\\evil.example/":                   "/",
		"https://evil.example/":              "/",
		"javascript:alert(1)":                "/",
		"relative/path":                      "/",
		"/auth/logout":                       "/",
		"/auth/logout?x=1":                   "/",
	}
	for next, want := range cases {
		require.Equal(t, want, loginRedirectTarget(next), "next=%q", next)
	}
}
