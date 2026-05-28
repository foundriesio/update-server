// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	toml "github.com/pelletier/go-toml"

	storage "github.com/foundriesio/dg-satellite/storage/gateway"
)

type ConfigFile = storage.ConfigFile

// @Summary Get device's current configuration
// @Produce json
// @Success 200 {object} map[string]ConfigFile
// @Router  /config [get]
func (h handlers) configGet(c echo.Context) error {
	req := c.Request()
	ctx := req.Context()
	log := CtxGetLog(ctx)
	d := CtxGetDevice(ctx)
	configs, timestamp, err := d.GetConfigs()
	if err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "failed to fetch config")
	} else if timestamp == 0 {
		return c.NoContent(http.StatusNoContent)
	}

	// All times below use one second precision to account for devices which don't support subsecond timestamps.
	// A client is expected to use the Date header value in its subsequent If-Modified-Since header values.
	cts := time.Unix(timestamp, 0).UTC()
	ifModifiedSince := req.Header.Get("If-Modified-Since")
	if len(ifModifiedSince) > 0 {
		if dts, err := time.Parse(time.RFC1123, ifModifiedSince); err != nil {
			log.Warn("Unable to parse If-Modified-Since", "error", err, "if-modified-since", ifModifiedSince)
		} else if !cts.After(dts) { // Latest update made at or before if-modified-since
			return c.String(http.StatusNotModified, "")
		}
	}

	// A reference type here allows manipulating map values directly below.
	applied := storage.AppliedConfigs{
		Files: make(map[string]ConfigFile),
	}
	pacmanCfg := make(pacmanConfig)
	for idx, srcConfig := range configs {
		var cfg map[string]ConfigFile
		if srcConfig == nil {
			continue
		}
		// If srcConfig is not nil, audit fields must be set, even if the Files are empty.
		// That's a valid use case if the entire global, group, or device configs were deleted.
		auditTrail := &applied.AuditTrail[idx]
		auditTrail.Reason = srcConfig.Reason
		auditTrail.CreatedAt = srcConfig.CreatedAt
		auditTrail.CreatedBy = srcConfig.CreatedBy
		if len(srcConfig.RawFiles) == 0 {
			continue
		} else if err = json.Unmarshal([]byte(srcConfig.RawFiles), &cfg); err != nil {
			return EchoError(c, err, http.StatusInternalServerError, "failed to parse config JSON")
		}
		for k, v := range cfg {
			if k == storage.ConfigSotaOverride {
				if err = pacmanCfg.merge(v.Value); err != nil {
					return EchoError(c, err, http.StatusInternalServerError, "failed to parse sota toml config")
				}
			}
			applied.Files[k] = v
		}
	}
	if !pacmanCfg.empty() {
		// When pacmanCfg is non-empty, files are warranted to contain the sota override.
		sotaCfg := applied.Files[storage.ConfigSotaOverride]
		if sotaCfg.Value, err = pacmanCfg.encode(); err != nil {
			return EchoError(c, err, http.StatusInternalServerError, "failed to encode merged sota toml config")
		} else {
			// set back into a map, as sotaCfg is a value copy
			applied.Files[storage.ConfigSotaOverride] = sotaCfg
		}
	}
	c.Response().Header().Set("Date", cts.Format(time.RFC1123))
	applied.AppliedAt = time.Now().Unix()
	if err := d.SaveAppliedConfigs(applied); err != nil {
		log.Warn("Failed to save applied config", "device", d.Uuid, "error", err)
	}
	return c.JSON(http.StatusOK, applied.Files)
}

type pacmanConfig map[string]map[string]interface{}

func (p pacmanConfig) empty() bool {
	return len(p) == 0
}

func (p pacmanConfig) encode() (string, error) {
	buf := new(bytes.Buffer)
	encoder := toml.NewEncoder(buf).Indentation("")
	if err := encoder.Encode(p); err != nil {
		return "", err
	}
	// pelletier/go-toml always adds a leading newline - trim it
	return strings.TrimLeft(buf.String(), "\n"), nil
}

func (p pacmanConfig) merge(tomlString string) error {
	var data pacmanConfig
	buf := bytes.NewReader([]byte(tomlString))
	decoder := toml.NewDecoder(buf)
	err := decoder.Decode(&data)
	if err != nil {
		return err
	}
	for section, values := range data {
		if p[section] == nil {
			p[section] = values
			continue
		}
		for k, v := range values {
			p[section][k] = v
		}
	}
	return nil
}
