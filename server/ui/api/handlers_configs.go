// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"

	"github.com/labstack/echo/v4"

	storage "github.com/foundriesio/dg-satellite/storage/api"
)

type (
	ConfigFile     = storage.ConfigFile
	ConfigFileSet  = storage.ConfigFileSet
	AppliedConfigs = storage.AppliedConfigs
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
		return c.JSON(http.StatusOK, configItemToJson(history[0]))
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
	user := CtxGetUser(c.Request().Context())
	if reason, cfg, err := validateConfigSet(c, true); err != nil {
		return err // EchoError is used internally
	} else if history, err := h.storage.ReadFactoryConfigHistory(1); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to read factory config history")
	} else if len(history) > 0 && string(cfg) == history[0].RawFiles {
		// No change - no need to create a new history item.
		return c.NoContent(http.StatusNoContent)
	} else if err = h.storage.SaveFactoryConfig(string(cfg), user.Username, reason); err != nil {
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
		return c.JSON(http.StatusOK, configItemToJson(history[0]))
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
	user := CtxGetUser(c.Request().Context())
	if !validateLabelValue(group) {
		err := fmt.Errorf("group value must match a given regexp: %s", validLabelValueRegex)
		return EchoError(c, err, http.StatusBadRequest, err.Error())
	} else if reason, cfg, err := validateConfigSet(c, false); err != nil {
		return err // EchoError is used internally
	} else if history, err := h.storage.ReadGroupConfigHistory(group, 1); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to read group config history")
	} else if len(history) > 0 && string(cfg) == history[0].RawFiles {
		// No change - no need to create a new history item.
		return c.NoContent(http.StatusNoContent)
	} else if err = h.storage.SaveGroupConfig(group, string(cfg), user.Username, reason); err != nil {
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
			return c.JSON(http.StatusOK, configItemToJson(history[0]))
		} else {
			return c.NoContent(http.StatusNoContent)
		}
	})
}

// @Summary Read the applied configuration last sent to a device
// @Description Returns the merged config (factory + group + device) that was most recently
// @Description delivered to the device, along with the Unix timestamp when it was applied.
// @Description Requires scopes: devices:read
// @Tags    Config
// @Produce json
// @Param   uuid path string true "Device UUID"
// @Success 200 {object} AppliedConfigs
// @Router  /configs/device/{uuid}/applied [get]
func (h *handlers) deviceAppliedConfigsGet(c echo.Context) error {
	return h.handleDevice(c, func(device *Device) error {
		envelope, err := h.storage.ReadAppliedConfigs(device.Uuid)
		if err != nil {
			return EchoError(c, err, http.StatusInternalServerError, "Failed to read applied config")
		}
		if envelope == nil {
			return c.NoContent(http.StatusNoContent)
		}
		return c.JSON(http.StatusOK, envelope)
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
		user := CtxGetUser(c.Request().Context())
		if reason, cfg, err := validateConfigSet(c, false); err != nil {
			return err // EchoError is used internally
		} else if history, err := h.storage.ReadDeviceConfigHistory(device.Uuid, 1); err != nil {
			return EchoError(c, err, http.StatusInternalServerError, "Failed to read device config history")
		} else if len(history) > 0 && string(cfg) == history[0].RawFiles {
			// No change - no need to create a new history item.
			return c.NoContent(http.StatusNoContent)
		} else if err = h.storage.SaveDeviceConfig(device.Uuid, string(cfg), user.Username, reason); err != nil {
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

type configFileSet struct {
	ConfigFileSet
	// Avoids unmarshalling Files into a map, and then marshalling them back.
	Files json.RawMessage `json:"Files"`
}

func configItemToJson(cfg *ConfigFileSet) *configFileSet {
	return &configFileSet{*cfg, json.RawMessage(cfg.RawFiles)}
}

func configHistoryToJson(history []*ConfigFileSet) []*configFileSet {
	res := make([]*configFileSet, len(history))
	for i, cfg := range history {
		res[i] = configItemToJson(cfg)
	}
	return res
}

var (
	// For a reason allow alphanum + basic punctuation + space. Important: no newlines.
	validConfigsReasonChars  = `a-zA-Z0-9_ ,.-:;'"`
	validConfigsReasonRegexp = `^[a-zA-Z0-9_ \,\.\-\:\;\'\"]*$`
	validateConfigsReason    = regexp.MustCompile(validConfigsReasonRegexp).MatchString
)

func validateConfigSet(c echo.Context, denySotaOverride bool) (reason string, content []byte, err error) {
	body := c.Request().Body
	defer body.Close() //nolint:errcheck
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	var configs ConfigFileSet
	if err = dec.Decode(&configs); err != nil {
		return "", nil, EchoError(c, err, http.StatusBadRequest, "Failed to parse request body")
	}
	const maxReasonLen = 200
	if len(configs.Reason) > maxReasonLen {
		err = fmt.Errorf("a maximum accepted reason length is %d", maxReasonLen)
		return "", nil, EchoError(c, err, http.StatusBadRequest, err.Error())
	} else if !validateConfigsReason(configs.Reason) {
		err = fmt.Errorf("reason must only contain the following characters: %s", validConfigsReasonChars)
		return "", nil, EchoError(c, err, http.StatusBadRequest, err.Error())
	}
	if denySotaOverride {
		if configs.Files == nil {
			configs.Files = make(map[string]ConfigFile, 0)
		} else if _, ok := configs.Files[storage.ConfigSotaOverride]; ok {
			err = fmt.Errorf("a '%s' is not allowed for global configs", storage.ConfigSotaOverride)
			return "", nil, EchoError(c, err, http.StatusBadRequest, err.Error())
		}
	}
	if content, err = json.Marshal(configs.Files); err != nil {
		return "", nil, EchoError(c, err, http.StatusInternalServerError, "Failed to save config files")
	}
	return configs.Reason, content, nil
}
