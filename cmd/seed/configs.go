// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"fmt"
	"log"

	"github.com/foundriesio/update-server/storage/api"
)

// seedGlobalConfigs saves a factory-wide config and one config per device
// group, so the Global/Group Configs pages aren't empty on a fresh server.
// Idempotent: skips any config class that already has history.
func seedGlobalConfigs(ap *api.Storage) error {
	if history, err := ap.ReadFactoryConfigHistory(1, false); err != nil {
		return fmt.Errorf("ReadFactoryConfigHistory: %w", err)
	} else if len(history) == 0 {
		const factoryConfig = `{"tag.main":{"Value":"main"}}`
		if err := ap.SaveFactoryConfig(factoryConfig, "noauth-fake-user", "seed"); err != nil {
			return fmt.Errorf("SaveFactoryConfig: %w", err)
		}
		log.Printf("create factory config")
	} else {
		log.Printf("skip  factory config (already exists)")
	}

	for _, group := range groups {
		if history, err := ap.ReadGroupConfigHistory(group, 1, false); err != nil {
			return fmt.Errorf("ReadGroupConfigHistory(%s): %w", group, err)
		} else if len(history) > 0 {
			log.Printf("skip  group config %s (already exists)", group)
			continue
		}
		groupConfig := fmt.Sprintf(`{"group.name":{"Value":"%s"}}`, group)
		if err := ap.SaveGroupConfig(group, groupConfig, "noauth-fake-user", "seed"); err != nil {
			return fmt.Errorf("SaveGroupConfig(%s): %w", group, err)
		}
		log.Printf("create group config %s", group)
	}
	return nil
}
