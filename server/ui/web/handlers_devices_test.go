// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"testing"

	"github.com/foundriesio/update-server/storage"
)

func boolPtr(b bool) *bool { return &b }

func TestBuildUpdateStepsHappyPath(t *testing.T) {
	events := []storage.DeviceUpdateEvent{
		{DeviceTime: "2026-08-30T09:00:27Z", Event: storage.DeviceEvent{Success: boolPtr(true)}},
		{DeviceTime: "2026-08-30T09:00:28Z", Event: storage.DeviceEvent{Success: boolPtr(true)}},
		{DeviceTime: "2026-08-30T09:00:31Z", Event: storage.DeviceEvent{Success: boolPtr(true)}},
	}

	steps, duration, failed := buildUpdateSteps(events)

	if failed {
		t.Errorf("failed = true, want false")
	}
	if duration != "4s" {
		t.Errorf("duration = %q, want 4s", duration)
	}
	want := []string{"+0s", "+1s", "+3s"}
	for i, w := range want {
		if steps[i].Elapsed != w {
			t.Errorf("steps[%d].Elapsed = %q, want %q", i, steps[i].Elapsed, w)
		}
	}
}

func TestBuildUpdateStepsMarksFailure(t *testing.T) {
	events := []storage.DeviceUpdateEvent{
		{DeviceTime: "2026-08-30T09:00:27Z", Event: storage.DeviceEvent{Success: boolPtr(true)}},
		{DeviceTime: "2026-08-30T09:00:28Z", Event: storage.DeviceEvent{Success: boolPtr(false)}},
	}

	steps, _, failed := buildUpdateSteps(events)

	if !failed {
		t.Errorf("failed = false, want true")
	}
	if steps[0].Failed {
		t.Errorf("steps[0].Failed = true, want false")
	}
	if !steps[1].Failed {
		t.Errorf("steps[1].Failed = false, want true")
	}
}

func TestBuildUpdateStepsClampsNegativeElapsed(t *testing.T) {
	events := []storage.DeviceUpdateEvent{
		{DeviceTime: "2026-08-30T09:00:27Z", Event: storage.DeviceEvent{Success: boolPtr(true)}},
		{DeviceTime: "2026-08-30T09:00:22Z", Event: storage.DeviceEvent{Success: boolPtr(true)}},
	}

	steps, duration, _ := buildUpdateSteps(events)

	if steps[1].Elapsed != "+0s" {
		t.Errorf("steps[1].Elapsed = %q, want +0s", steps[1].Elapsed)
	}
	if duration != "0s" {
		t.Errorf("duration = %q, want 0s", duration)
	}
}

func TestBuildUpdateStepsNilSuccessIsNotFailure(t *testing.T) {
	events := []storage.DeviceUpdateEvent{
		{DeviceTime: "2026-08-30T09:00:27Z", Event: storage.DeviceEvent{}},
	}

	steps, duration, failed := buildUpdateSteps(events)

	if failed {
		t.Errorf("failed = true, want false")
	}
	if steps[0].Failed {
		t.Errorf("steps[0].Failed = true, want false")
	}
	if duration != "0s" {
		t.Errorf("duration = %q, want 0s", duration)
	}
}
