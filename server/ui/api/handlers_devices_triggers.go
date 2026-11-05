// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/foundriesio/update-server/server"
	"github.com/foundriesio/update-server/storage"
	"github.com/labstack/echo/v4"
)

type RemoteActions struct {
	Actions []string `json:"actions"`
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
	const randomIDLength = 15
	const maxActionLength = 48 - randomIDLength - 1
	return len(action) <= maxActionLength && storage.TestIdRegex.MatchString(action+"_"+strings.Repeat("a", randomIDLength))
}

func (h *handlers) deviceRemoteActions(c echo.Context) error {
	return h.handleDevice(c, func(device *Device) error {
		configs, err := h.deviceConfigFiles(c, device.Uuid)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, RemoteActions{Actions: remoteActions(configs)})
	})
}

func (h *handlers) deviceRemoteActionTrigger(c echo.Context) error {
	return h.handleDevice(c, func(device *Device) error {
		var req RemoteActionTrigger
		dec := json.NewDecoder(c.Request().Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			return server.EchoError(c, err, http.StatusBadRequest, "Failed to parse request body")
		}
		var extra any
		if err := dec.Decode(&extra); err != io.EOF {
			return server.EchoError(c, err, http.StatusBadRequest, "Failed to parse request body")
		}

		configs, err := h.deviceConfigFiles(c, device.Uuid)
		if err != nil {
			return err
		}
		if !slices.Contains(remoteActions(configs), req.Action) {
			return echo.NewHTTPError(http.StatusBadRequest, "Remote action is not configured for this device")
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
