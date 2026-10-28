// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package templates

import (
	"testing"
	"time"
)

var testNow = time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)

func TestRelativeTime(t *testing.T) {
	ago := func(d time.Duration) int64 {
		return testNow.Add(-d).Unix()
	}

	tests := []struct {
		name string
		ts   int64
		want string
	}{
		{"zero", 0, "Never"},
		{"negative", -1, "Never"},
		{"future", ago(-time.Hour), "just now"},
		{"59s", ago(59 * time.Second), "just now"},
		{"60s", ago(60 * time.Second), "1 minute ago"},
		{"5m", ago(5 * time.Minute), "5 minutes ago"},
		{"59m", ago(59 * time.Minute), "59 minutes ago"},
		{"1h", ago(time.Hour), "1 hour ago"},
		{"23h", ago(23 * time.Hour), "23 hours ago"},
		{"24h", ago(24 * time.Hour), "1 day ago"},
		{"6d23h", ago(6*day + 23*time.Hour), "6 days ago"},
		{"7d", ago(7 * day), "Oct 21, 2026"},
		{"30d", ago(30 * day), "Sep 28, 2026"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RelativeTime(testNow, tt.ts); got != tt.want {
				t.Errorf("RelativeTime() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatDate(t *testing.T) {
	tests := []struct {
		name string
		when time.Time
		want string
	}{
		{"typical", testNow, "Oct 28, 2026"},
		{"single digit day", time.Date(2026, time.January, 2, 12, 0, 0, 0, time.UTC), "Jan 2, 2026"},
		{"year end", time.Date(2025, time.December, 31, 23, 59, 59, 0, time.UTC), "Dec 31, 2025"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatDate(tt.when.Unix()); got != tt.want {
				t.Errorf("FormatDate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatDateIgnoresLocalZone(t *testing.T) {
	local := time.Local
	time.Local = time.FixedZone("UTC-5", -5*60*60)
	defer func() { time.Local = local }()

	// 00:30 UTC on Jan 1 is still Dec 31 in UTC-5.
	ts := time.Date(2026, time.January, 1, 0, 30, 0, 0, time.UTC).Unix()
	if got, want := FormatDate(ts), "Jan 1, 2026"; got != want {
		t.Errorf("FormatDate() = %q, want %q", got, want)
	}
}
