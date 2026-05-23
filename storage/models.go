// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"regexp"
)

// DeviceUpdateEvent represents update events that devices send the
// device-gateway.
type DeviceUpdateEvent struct {
	Id         string          `json:"id"`
	DeviceTime string          `json:"deviceTime"`
	Event      DeviceEvent     `json:"event"`
	EventType  DeviceEventType `json:"eventType"`
}

var ValidCorrelationId = regexp.MustCompile(`^[a-zA-Z0-9_\-]+$`).MatchString

type DeviceEvent struct {
	CorrelationId string `json:"correlationId"`
	Ecu           string `json:"ecu"`
	Success       *bool  `json:"success,omitempty"`
	TargetName    string `json:"targetName"`
	Version       string `json:"version"`
	Details       string `json:"details,omitempty"`
}

type DeviceEventType struct {
	Id      string `json:"id"`
	Version int    `json:"version"`
}

type DeviceStatus struct {
	Uuid          string `json:"uuid"`
	CorrelationId string `json:"correlationId"`
	TargetName    string `json:"target-name"`
	Status        string `json:"status"`
	DeviceTime    string `json:"deviceTime"`
}

type ConfigFile struct {
	Value       string   `json:"Value"`
	Unencrypted *bool    `json:"Unencrypted,omitempty"`
	OnChanged   []string `json:"OnChanged,omitempty"`
}

type ConfigFileSet struct {
	// Storage returns RawFiles, but API returns parsed Files.
	RawFiles  string                `json:"-"`
	Files     map[string]ConfigFile `json:"Files"`
	Reason    string                `json:"Reason,omitempty"`
	CreatedAt int64                 `json:"CreatedAt,omitempty"`
	CreatedBy string                `json:"CreatedBy,omitempty"`
}

// AppliedConfigs wraps the merged config sent to a device along with
// the Unix timestamp (seconds) at which it was delivered.
type AppliedConfigs struct {
	Files     map[string]ConfigFile `json:"config"`
	AppliedAt int64                 `json:"applied_at"`
}

var evtIdToStatus = map[string]string{
	"MetadataUpdateCompleted":  "Metadata update completed",
	"EcuDownloadStarted":       "Download started",
	"EcuDownloadCompleted":     "Download completed",
	"EcuInstallationStarted":   "Installation started",
	"EcuInstallationApplied":   "Installation applied",
	"EcuInstallationCompleted": "Installation completed",
	"CertRotationStarted":      "Certificate rotation started",
	"CertRotationCompleted":    "Certificate rotation completed",
}

func (e DeviceUpdateEvent) ParseStatus() DeviceStatus {
	var status string

	if _, ok := evtIdToStatus[e.EventType.Id]; ok {
		status = evtIdToStatus[e.EventType.Id]
	} else {
		status = "Unknown event type: " + e.EventType.Id
	}

	switch e.EventType.Id {
	case "EcuInstallationApplied":
		status += "; awaiting update finalization"
	case "EcuDownloadCompleted", "EcuInstallationCompleted", "CertRotationCompleted", "MetadataUpdateCompleted":
		if e.Event.Success != nil {
			if !*e.Event.Success {
				status += "; failed"
			} else {
				status += "; succeeded"
			}
		} else {
			status += "; unknown result"
		}
	}

	return DeviceStatus{
		CorrelationId: e.Event.CorrelationId,
		TargetName:    e.Event.TargetName,
		Status:        status,
		DeviceTime:    e.DeviceTime,
	}
}

type AppsStates struct {
	DeviceTime string `json:"deviceTime"`
	Ostree     string `json:"ostree"`
	Apps       map[string]struct {
		Uri      string `json:"uri"`
		State    string `json:"state"`
		Services []struct {
			Name     string `json:"name"`
			Hash     string `json:"hash"`
			Health   string `json:"health,omitempty"`
			ImageUri string `json:"image"`
			Logs     string `json:"logs,omitempty"`
			State    string `json:"state"`
			Status   string `json:"status"`
		} `json:"services"`
	} `json:"apps"`
}

var TestIdRegex = regexp.MustCompile(`^[A-Za-z0-9\-\_]{15,48}$`)

type TargetTestResult struct {
	Name    string             `json:"name"`
	Status  string             `json:"status"`
	LocalTs float64            `json:"local_ts"`
	Details string             `json:"details"`
	Metrics map[string]float64 `json:"metrics"`
}

type TargetTest struct {
	Uuid        string             `json:"uuid"`
	Name        string             `json:"name"`
	TargetName  string             `json:"target_name"`
	Status      string             `json:"status"`
	CreatedOn   int64              `json:"created_on"`
	CompletedOn *int64             `json:"completed_on"`
	Details     string             `json:"details,omitempty"`
	Artifacts   []string           `json:"artifacts,omitempty"`
	Results     []TargetTestResult `json:"results,omitempty"`
}
