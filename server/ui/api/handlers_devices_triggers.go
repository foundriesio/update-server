// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/foundriesio/update-server/server"
	"github.com/foundriesio/update-server/storage"
	"github.com/labstack/echo/v4"
)

type RemoteActions struct {
	Actions     []string            `json:"actions"`
	Outstanding []RemoteActionState `json:"outstanding,omitempty"`
}

type RemoteActionState struct {
	Action string `json:"action"`
	TestID string `json:"test_id"`
	State  string `json:"state"`
}

type RemoteActionTrigger struct {
	Action string `json:"action" form:"action"`
}

type RemoteActionTriggerResult struct {
	TestID string `json:"test_id"`
}

func remoteActions(configs map[string]storage.ConfigFile) []string {
	known := make(map[string]struct{})
	for action := range strings.SplitSeq(configs["fio-remote-actions"].Value, ",") {
		action = strings.TrimSpace(action)
		if action != "" && validRemoteAction(action) {
			known[action] = struct{}{}
		}
	}
	actions := make([]string, 0, len(known))
	for action := range known {
		actions = append(actions, action)
	}
	sort.Strings(actions)
	return actions
}

func validRemoteAction(action string) bool {
	return storage.TestIdRegex.MatchString(action + "_123456789abcdef")
}

// @Summary Get configured device remote actions
// @Description Requires scopes: devices:read
// @Tags    Devices
// @Produce json
// @Success 200 {object} RemoteActions
// @Param   uuid path string true "Device UUID"
// @Router  /devices/{uuid}/remote-actions [get]
func (h *handlers) deviceRemoteActions(c echo.Context) error {
	return h.handleDevice(c, func(device *Device) error {
		configs, err := h.deviceConfigFiles(c, device.Uuid)
		if err != nil {
			return err
		}
		commands, err := remoteActionCommands(configs)
		if err != nil {
			return server.EchoError(c, err, http.StatusInternalServerError, "Failed to read remote action commands")
		}
		result := RemoteActions{Actions: remoteActions(configs)}
		if len(commands) > 0 {
			applied, err := h.storage.ReadAppliedConfigs(device.Uuid)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return server.EchoError(c, err, http.StatusInternalServerError, "Failed to read delivered remote actions")
			}
			tests, err := device.GetTests()
			if err != nil {
				return server.EchoError(c, err, http.StatusInternalServerError, "Failed to read remote action tests")
			}
			result.Outstanding = outstandingRemoteActions(commands, configs, applied, tests)
		}
		return c.JSON(http.StatusOK, result)
	})
}

// @Summary Run a device remote action
// @Description Requires scopes: devices:read-update, updates:read-update
// @Tags    Devices
// @Accept  json
// @Produce json
// @Param   uuid path string true "Device UUID"
// @Param   data body RemoteActionTrigger true "Remote action to run"
// @Success 200 {object} RemoteActionTriggerResult
// @Router  /devices/{uuid}/remote-actions [post]
func (h *handlers) deviceRemoteActionTrigger(c echo.Context) error {
	return h.handleDevice(c, func(device *Device) error {
		var req RemoteActionTrigger
		if err := c.Bind(&req); err != nil {
			return server.EchoError(c, err, http.StatusBadRequest, "Failed to parse request body")
		}

		configs, err := h.deviceConfigFiles(c, device.Uuid)
		if err != nil {
			return err
		}
		if !slices.Contains(remoteActions(configs), req.Action) {
			return server.EchoError(c, nil, http.StatusBadRequest, fmt.Sprintf("Remote action %q is not configured for this device", req.Action))
		}

		commandID := req.Action + "_" + rand.Text()[:15]
		configs["fioconfig-oneshot-"+req.Action] = storage.ConfigFile{
			Value:       fmt.Sprintf("COMMAND_ID=%s\nCOMMAND_CAPTURE=1\n", commandID),
			Unencrypted: true,
			OnChanged:   []string{"/usr/share/fioconfig/handlers/fioconfig-oneshot", req.Action},
		}
		content, err := json.Marshal(configs)
		if err != nil {
			return server.EchoError(c, err, http.StatusInternalServerError, "Failed to save remote action")
		}
		user := CtxGetUser(c.Request().Context())
		if err := h.storage.SaveDeviceConfig(device.Uuid, string(content), user.Username, "Run remote action: "+req.Action); err != nil {
			return server.EchoError(c, err, http.StatusInternalServerError, "Failed to save remote action")
		}
		return c.JSON(http.StatusOK, RemoteActionTriggerResult{TestID: commandID})
	})
}

func remoteActionCommands(configs map[string]storage.ConfigFile) ([]RemoteActionState, error) {
	var names []string
	for name := range configs {
		if strings.HasPrefix(name, "fioconfig-oneshot-") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var commands []RemoteActionState
	for _, name := range names {
		action := strings.TrimPrefix(name, "fioconfig-oneshot-")
		var id string
		for line := range strings.SplitSeq(configs[name].Value, "\n") {
			if value, ok := strings.CutPrefix(line, "COMMAND_ID="); ok {
				id = value
				break
			}
		}
		if action == "" || !validRemoteAction(action) || !storage.TestIdRegex.MatchString(id) ||
			!strings.HasPrefix(id, action+"_") || len(id) != len(action)+16 {
			return nil, fmt.Errorf("invalid remote action command in %s", name)
		}
		commands = append(commands, RemoteActionState{Action: action, TestID: id})
	}
	return commands, nil
}

func outstandingRemoteActions(commands []RemoteActionState, configs map[string]storage.ConfigFile, applied *storage.AppliedConfigs, tests []storage.TargetTest) []RemoteActionState {
	reported := make(map[string]bool, len(tests))
	for _, test := range tests {
		reported[test.Uuid] = true
	}
	var outstanding []RemoteActionState
	for _, command := range commands {
		// A reported test takes precedence even when delivery and test reads overlap.
		if reported[command.TestID] {
			continue
		}
		command.State = "pending"
		if applied != nil {
			name := "fioconfig-oneshot-" + command.Action
			want, delivered := configs[name], applied.Files[name]
			if want.Value == delivered.Value && want.Unencrypted == delivered.Unencrypted && slices.Equal(want.OnChanged, delivered.OnChanged) {
				command.State = "queued"
			}
		}
		outstanding = append(outstanding, command)
	}
	return outstanding
}

func (h *handlers) deviceConfigFiles(c echo.Context, uuid string) (map[string]storage.ConfigFile, error) {
	history, err := h.storage.ReadDeviceConfigHistory(uuid, 1, true)
	if err != nil {
		return nil, server.EchoError(c, err, http.StatusInternalServerError, "Failed to read device config")
	}
	configs := make(map[string]storage.ConfigFile)
	if len(history) > 0 {
		if err := json.Unmarshal([]byte(history[0].RawFiles), &configs); err != nil {
			return nil, server.EchoError(c, err, http.StatusInternalServerError, "Failed to read device config")
		}
	}
	return configs, nil
}
