// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/labstack/echo/v4"

	storage "github.com/foundriesio/dg-satellite/storage/gateway"
)

const sotaOverride = "z-50-fioctl.toml"

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
	cts := time.Unix(timestamp, 0)
	ifModifiedSince := req.Header.Get("If-Modified-Since")
	if len(ifModifiedSince) > 0 {
		if dts, err := time.Parse(time.RFC1123, ifModifiedSince); err != nil {
			log.Warn("Unable to parse If-Modified-Since", "error", err, "if-modified-since", ifModifiedSince)
		} else if !cts.After(dts) { // Latest update made at or before if-modified-since
			return c.String(http.StatusNotModified, "")
		}
	}

	// A reference type here allows manipulating map values directly below.
	files := make(map[string]*ConfigFile)
	pacmanCfg := make(pacmanConfig)
	for _, rawConfig := range configs {
		var cfg map[string]*ConfigFile
		if len(rawConfig) == 0 {
			continue
		} else if err = json.Unmarshal([]byte(rawConfig), &cfg); err != nil {
			return EchoError(c, err, http.StatusInternalServerError, "failed to parse config JSON")
		}
		for k, v := range cfg {
			if k == sotaOverride {
				if err = pacmanCfg.merge(v.Value); err != nil {
					return EchoError(c, err, http.StatusInternalServerError, "failed to parse sota toml config")
				}
			}
			files[k] = v
		}
	}
	if !pacmanCfg.empty() {
		if files[sotaOverride].Value, err = pacmanCfg.encode(); err != nil {
			return EchoError(c, err, http.StatusInternalServerError, "failed to encode merged sota toml config")
		}
	}
	c.Response().Header().Set("Date", cts.Format(time.RFC1123))
	return c.JSON(http.StatusOK, files)
}

type pacmanConfig map[string]map[string]interface{}

func (p pacmanConfig) empty() bool {
	return len(p) == 0
}

func (p pacmanConfig) encode() (string, error) {
	buf := new(bytes.Buffer)
	encoder := toml.NewEncoder(buf)
	encoder.Indent = ""
	if err := encoder.Encode(p); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (p pacmanConfig) merge(tomlString string) error {
	var data pacmanConfig
	err := toml.Unmarshal([]byte(tomlString), &data)
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
