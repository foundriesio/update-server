// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"

	"github.com/labstack/echo/v4"

	storage "github.com/foundriesio/dg-satellite/storage/api"
)

type (
	ConfigFile    = storage.ConfigFile
	ConfigFileSet = map[string]ConfigFile
)

const ConfigHistoryLimit = storage.ConfigHistoryLimit

// @Summary Read latest factory configs
// @Description Requires scopes: devices:read
// @Tags    Config
// @Produce json
// @Success 200 {object} ConfigFileSet
// @Router  /configs/factory [get]
func (h *handlers) factoryConfigsGet(c echo.Context) error {
	if history, err := h.storage.ReadFactoryConfigHistory(1); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to read factory config history")
	} else if len(history) > 0 {
		return c.JSON(http.StatusOK, json.RawMessage(history[0]))
	} else {
		return c.NoContent(http.StatusNoContent)
	}
}

// @Summary Read factory configs history
// @Description Requires scopes: devices:read
// @Tags    Config
// @Produce json
// @Success 200 {array} ConfigFileSet
// @Router  /configs/factory/history [get]
func (h *handlers) factoryConfigsHistory(c echo.Context) error {
	if history, err := h.storage.ReadFactoryConfigHistory(ConfigHistoryLimit); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to read factory config history")
	} else {
		return c.JSON(http.StatusOK, configHistoryToJson(history))
	}
}

// @Summary Save factory configs, replacing current value
// @Description Requires scopes: devices:read-update, updates:read-update
// @Tags    Config
// @Accept  json
// @Param   data body ConfigFileSet true "Factory config files"
// @Success 204
// @Router  /configs/factory [put]
func (h *handlers) factoryConfigsPut(c echo.Context) error {
	if cfg, err := validateConfigSet(c, true); err != nil {
		return err // EchoError is used internally
	} else if history, err := h.storage.ReadFactoryConfigHistory(1); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to read factory config history")
	} else if len(history) > 0 && string(cfg) == history[0] {
		// No change - no need to create a new history item.
		return c.NoContent(http.StatusNoContent)
	} else if err = h.storage.SaveFactoryConfig(string(cfg)); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to save factory config history")
	} else {
		// New history item created.
		return c.NoContent(http.StatusNoContent)
	}
}

// @Summary Read latest group configs
// @Description Requires scopes: devices:read
// @Tags    Config
// @Produce json
// @Param   name path string true "Group name"
// @Success 200 {object} ConfigFileSet
// @Router  /configs/group/{name} [get]
func (h *handlers) groupConfigsGet(c echo.Context) error {
	group := c.Param("name")
	if history, err := h.storage.ReadGroupConfigHistory(group, 1); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to read group config history")
	} else if len(history) > 0 {
		return c.JSON(http.StatusOK, json.RawMessage(history[0]))
	} else if knownGroups, err := h.storage.GetKnownDeviceGroupNames(); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to get known group names")
	} else if slices.Contains(knownGroups, group) {
		return c.NoContent(http.StatusNoContent)
	} else {
		return c.NoContent(http.StatusNotFound)
	}
}

// @Summary Read group configs history
// @Description Requires scopes: devices:read
// @Tags    Config
// @Produce json
// @Param   name path string true "Group name"
// @Success 200 {array} ConfigFileSet
// @Router  /configs/group/{name}/history [get]
func (h *handlers) groupConfigsHistory(c echo.Context) error {
	group := c.Param("name")
	if history, err := h.storage.ReadGroupConfigHistory(group, ConfigHistoryLimit); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to read group config history")
	} else if len(history) > 0 {
		return c.JSON(http.StatusOK, configHistoryToJson(history))
	} else if knownGroups, err := h.storage.GetKnownDeviceGroupNames(); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to get known group names")
	} else if slices.Contains(knownGroups, group) {
		return c.JSON(http.StatusOK, configHistoryToJson(nil))
	} else {
		return c.NoContent(http.StatusNotFound)
	}
}

// @Summary Save group configs, replacing current value
// @Description Requires scopes: devices:read-update, updates:read-update
// @Tags    Config
// @Accept  json
// @Param   name path string true "Group name"
// @Param   data body ConfigFileSet true "Factory config files"
// @Success 204
// @Router  /configs/group/{name} [put]
func (h *handlers) groupConfigsPut(c echo.Context) error {
	group := c.Param("name")
	if !validateLabelValue(group) {
		err := fmt.Errorf("group value must match a given regexp: %s", validLabelValueRegex)
		return EchoError(c, err, http.StatusBadRequest, err.Error())
	} else if cfg, err := validateConfigSet(c, false); err != nil {
		return err // EchoError is used internally
	} else if history, err := h.storage.ReadGroupConfigHistory(group, 1); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to read group config history")
	} else if len(history) > 0 && string(cfg) == history[0] {
		// No change - no need to create a new history item.
		return c.NoContent(http.StatusNoContent)
	} else if err = h.storage.SaveGroupConfig(group, string(cfg)); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to save group config history")
	} else {
		// New history item created.
		return c.NoContent(http.StatusNoContent)
	}
}

// @Summary Read latest device configs
// @Description Requires scopes: devices:read
// @Tags    Config
// @Produce json
// @Param   uuid path string true "Device uuid"
// @Success 200 {object} ConfigFileSet
// @Router  /configs/device/{uuid} [get]
func (h *handlers) deviceConfigsGet(c echo.Context) error {
	return h.handleDevice(c, func(device *Device) error {
		if history, err := h.storage.ReadDeviceConfigHistory(device.Uuid, 1); err != nil {
			return EchoError(c, err, http.StatusInternalServerError, "Failed to read device config history")
		} else if len(history) > 0 {
			return c.JSON(http.StatusOK, json.RawMessage(history[0]))
		} else {
			return c.NoContent(http.StatusNoContent)
		}
	})
}

// @Summary Read device configs history
// @Description Requires scopes: devices:read
// @Tags    Config
// @Produce json
// @Param   uuid path string true "Device uuid"
// @Success 200 {array} ConfigFileSet
// @Router  /configs/device/{uuid}/history [get]
func (h *handlers) deviceConfigsHistory(c echo.Context) error {
	return h.handleDevice(c, func(device *Device) error {
		if history, err := h.storage.ReadDeviceConfigHistory(device.Uuid, ConfigHistoryLimit); err != nil {
			return EchoError(c, err, http.StatusInternalServerError, "Failed to read device config history")
		} else {
			return c.JSON(http.StatusOK, configHistoryToJson(history))
		}
	})
}

// @Summary Save device configs, replacing current value
// @Description Requires scopes: devices:read-update, updates:read-update
// @Tags    Config
// @Accept  json
// @Param   uuid path string true "Device uuid"
// @Param   data body ConfigFileSet true "Factory config files"
// @Success 204
// @Router  /configs/device/{uuid} [put]
func (h *handlers) deviceConfigsPut(c echo.Context) error {
	return h.handleDevice(c, func(device *Device) error {
		if cfg, err := validateConfigSet(c, false); err != nil {
			return err // EchoError is used internally
		} else if history, err := h.storage.ReadDeviceConfigHistory(device.Uuid, 1); err != nil {
			return EchoError(c, err, http.StatusInternalServerError, "Failed to read device config history")
		} else if len(history) > 0 && string(cfg) == history[0] {
			// No change - no need to create a new history item.
			return c.NoContent(http.StatusNoContent)
		} else if err = h.storage.SaveDeviceConfig(device.Uuid, string(cfg)); err != nil {
			return EchoError(c, err, http.StatusInternalServerError, "Failed to save device config history")
		} else {
			// New history item created.
			return c.NoContent(http.StatusNoContent)
		}
	})
}

// @Summary Upload factory/group/device configs from an archive
// @Description Requires scopes: devices:read-update, updates:read-update
// @Tags    Config
// @Accept  application/x-tar
// @Success 200
// @Router  /configs [put]
func (h *handlers) configsUpload(c echo.Context) error {
	req := c.Request()

	payload := req.Body
	defer payload.Close() //nolint:errcheck

	var brokenErr *storage.ErrConfigUploadBroken
	if err := h.storage.UploadConfigs(payload); err == nil {
		return c.String(http.StatusOK, "Configs uploaded successfully")
	} else if errors.As(err, &brokenErr) {
		// This is practically impossible.
		// But if it happens - there is a problem at filesystem level, and the user must intervene.
		CtxGetLog(req.Context()).Error("configs folder is broken by upload", "upload", brokenErr.UploadPath, "error", err)
		return c.String(http.StatusServiceUnavailable, fmt.Sprintf(`
Configs upload broke the configs directory.
Neither old nor new configs are now active.
It can be fixed by uploading the same file again.
If an error persists, a problem needs to be fixed manually.
Inspect the contents of '%s' where both the uploaded and backup configs are stored.
One of them should be moved to the configs directory at '%s'.`,
			brokenErr.UploadPath,
			brokenErr.ConfigsPath,
		))

	} else {
		return EchoError(c, err, http.StatusInternalServerError, "Configs upload failed")
	}
}

func configHistoryToJson(history []string) []json.RawMessage {
	res := make([]json.RawMessage, len(history))
	for i, cfg := range history {
		res[i] = json.RawMessage(cfg)
	}
	return res
}

func validateConfigSet(c echo.Context, denySotaOverride bool) ([]byte, error) {
	// We only need to validate config files, and return the original body (save on serialization)
	body := c.Request().Body
	defer body.Close() //nolint:errcheck
	res, err := io.ReadAll(body)
	if err != nil {
		return nil, EchoError(c, err, http.StatusBadRequest, "Failed to read request body")
	}
	dec := json.NewDecoder(bytes.NewReader(res))
	dec.DisallowUnknownFields()
	var configs ConfigFileSet
	if err = dec.Decode(&configs); err != nil {
		return nil, EchoError(c, err, http.StatusBadRequest, "Failed to parse request body")
	}
	if denySotaOverride {
		if _, ok := configs[storage.ConfigSotaOverride]; ok {
			err = fmt.Errorf("a '%s' is not allowed for global configs", storage.ConfigSotaOverride)
			return nil, EchoError(c, err, http.StatusBadRequest, err.Error())
		}
	}
	return res, nil
}
