// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"errors"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"github.com/foundriesio/update-server/server"
	storage "github.com/foundriesio/update-server/storage/api"
	"github.com/labstack/echo/v4"
)

type TargetTest = storage.TargetTest

// @Summary Get device tests
// @Tags    Devices
// @Produce json
// @Success 200 {array} TargetTest
// @Param   uuid path string true "Device UUID"
// @Router  /devices/{uuid}/tests [get]
func (h *handlers) deviceTestsList(c echo.Context) error {
	return h.handleDevice(c, func(device *Device) error {
		deviceTests, err := device.GetTests()
		if err != nil {
			return server.EchoError(c, err, http.StatusInternalServerError, "Failed to lookup test")
		}

		return c.JSON(http.StatusOK, deviceTests)
	})
}

// @Summary Get device test
// @Tags    Devices
// @Produce json
// @Success 200 {object} TargetTest
// @Param   uuid path string true "Device UUID"
// @Param   testid path string true "Test ID"
// @Router  /devices/{uuid}/tests/{testid} [get]
func (h *handlers) deviceTestGet(c echo.Context) error {
	return h.handleDevice(c, func(device *Device) error {
		testid := c.Param("testid")
		if !storage.TestIdRegex.MatchString(testid) {
			return echo.NewHTTPError(http.StatusNotFound, "Test not found")
		}

		test, err := device.GetTest(testid)
		if err != nil {
			return server.EchoError(c, err, http.StatusInternalServerError, "Failed to lookup test")
		} else if test == nil {
			return echo.NewHTTPError(http.StatusNotFound, "Test not found")
		}

		return c.JSON(http.StatusOK, test)
	})
}

// @Summary Get device test artifact
// @Tags    Devices
// @Produce octet-stream
// @Success 200 {file} binary
// @Failure 404 "Test or artifact not found"
// @Param   uuid path string true "Device UUID"
// @Param   testid path string true "Test ID"
// @Param   artifact path string true "Artifact name"
// @Router  /devices/{uuid}/tests/{testid}/{artifact} [get]
func (h *handlers) deviceTestArtifact(c echo.Context) error {
	return h.handleDevice(c, func(device *Device) error {
		testid := c.Param("testid")
		artifact := c.Param("artifact")
		log := CtxGetLog(c.Request().Context())

		if !storage.TestIdRegex.MatchString(testid) {
			return echo.NewHTTPError(http.StatusNotFound, "Test not found")
		}

		fd, err := device.GetTestArtifact(testid, artifact)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return c.NoContent(http.StatusNotFound)
			}
			return server.EchoError(c, err, http.StatusInternalServerError, "Failed to lookup artifact")
		}
		defer func() {
			if err := fd.Close(); err != nil {
				log.Error("Failed to close artifact file", "error", err)
			}
		}()
		contentType := mime.TypeByExtension(filepath.Ext(artifact))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		return c.Stream(http.StatusOK, contentType, fd)
	})
}
