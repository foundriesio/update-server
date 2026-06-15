// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear
package gateway

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	storage "github.com/foundriesio/update-server/storage/gateway"
)

// @Summary Set aktualizr-lites's running configuration
// @Accept  application/toml
// @Produce plain
// @Success 200 ""
// @Router  /system_info/config [put]
func (handlers) akTomlInfo(c echo.Context) error {
	d := CtxGetDevice(c.Request().Context())
	if bytes, err := ReadBody(c); err != nil {
		return err
	} else if err = d.PutFile(storage.AktomlFile, string(bytes)); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to save aktoml")
	} else {
		return c.String(http.StatusOK, "")
	}
}

// @Summary Set the hardware info of a device
// @Accept  json
// @Produce plain
// @Success 200 ""
// @Router  /system_info [put]
func (handlers) hardwareInfo(c echo.Context) error {
	// Loosely verify hardware info JSON and save original payload
	var data interface{}
	d := CtxGetDevice(c.Request().Context())
	if bytes, err := ReadBody(c); err != nil {
		return err
	} else if err = ParseJsonBody(c, bytes, &data); err != nil {
		return err
	} else if err = d.PutFile(storage.HwInfoFile, string(bytes)); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to save hwinfo")
	} else {
		return c.String(http.StatusOK, "")
	}
}

// @Summary Set the network info of a device
// @Accept  json
// @Param   data body NetworkInfo true "Network Info"
// @Produce plain
// @Success 200 ""
// @Router  /system_info/network [put]
func (handlers) networkInfo(c echo.Context) error {
	// Strictly verify network info JSON and save original payload
	var data NetworkInfo
	d := CtxGetDevice(c.Request().Context())
	if bytes, err := ReadBody(c); err != nil {
		return err
	} else if err = ParseJsonBody(c, bytes, &data); err != nil {
		return err
	} else if err = d.PutFile(storage.NetInfoFile, string(bytes)); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to save netinfo")
	} else {
		return c.String(http.StatusOK, "")
	}
}

// @Summary Store the apps states info of a device
// @Accept  json
// @Param   data body AppsStates true "Apps States"
// @Produce plain
// @Success 200 ""
// @Router  /apps-states [post]
func (handlers) appsStatesInfo(c echo.Context) error {
	// Strictly verify apps states info JSON and save original payload
	var data AppsStates
	d := CtxGetDevice(c.Request().Context())
	if bytes, err := ReadBody(c); err != nil {
		return err
	} else if err = ParseJsonBody(c, bytes, &data); err != nil {
		return err
	} else if _, err := time.Parse(time.RFC3339, data.DeviceTime); err != nil {
		msg := fmt.Sprintf("Failed to parse device time, must be RFC3339: %s", data.DeviceTime)
		return EchoError(c, err, http.StatusBadRequest, msg)
	} else if err = d.SaveAppsStates(string(bytes)); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to save apps-states")
	} else {
		return c.String(http.StatusOK, "")
	}
}
