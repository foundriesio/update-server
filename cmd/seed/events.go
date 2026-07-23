// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"fmt"
	"time"

	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/gateway"
)

// seedRolloutEvents sends a fake sequence of update-progress events for each
// device in uuids, so the device's update-history page and the rollout tail
// log show real content instead of being empty.
func seedRolloutEvents(gw *gateway.Storage, updateName string, uuids []string) error {
	stages := []string{
		"EcuDownloadStarted",
		"EcuDownloadCompleted",
		"EcuInstallationStarted",
		"EcuInstallationApplied",
		"EcuInstallationCompleted",
	}

	for _, uuid := range uuids {
		d, err := gw.DeviceGet(uuid)
		if err != nil {
			return fmt.Errorf("DeviceGet(%s): %w", uuid, err)
		}
		if d == nil {
			continue
		}

		targetName := fmt.Sprintf("intel-corei7-64-lmp-%s", updateName)
		corrId := fmt.Sprintf("seed-rollout-%s", uuid)
		now := time.Now().UTC()

		events := make([]storage.DeviceUpdateEvent, 0, len(stages))
		for i, stage := range stages {
			success := true
			events = append(events, storage.DeviceUpdateEvent{
				Id:         fmt.Sprintf("%s-%d", corrId, i),
				DeviceTime: now.Add(time.Duration(i) * time.Second).Format(time.RFC3339),
				Event: storage.DeviceEvent{
					CorrelationId: corrId,
					Ecu:           "seed-ecu",
					Success:       &success,
					TargetName:    targetName,
					Version:       updateName,
				},
				EventType: storage.DeviceEventType{Id: stage, Version: 1},
			})
		}
		if err := d.ProcessEvents(events); err != nil {
			return fmt.Errorf("ProcessEvents(%s): %w", uuid, err)
		}
	}
	return nil
}
