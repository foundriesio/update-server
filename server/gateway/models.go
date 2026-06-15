// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package gateway

import (
	storage "github.com/foundriesio/update-server/storage/gateway"
)

type (
	AppsStates  = storage.AppsStates
	Device      = storage.Device
	UpdateEvent = storage.DeviceUpdateEvent
)

type NetworkInfo struct {
	Hostname  string `json:"hostname,omitempty"`
	Mac       string `json:"mac,omitempty"`
	LocalIpv4 string `json:"local_ipv4,omitempty"`
}
