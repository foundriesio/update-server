// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

const rootJsonSuffix = ".root.json"

// @Summary Returns the latest TUF root metadata
// @Description Requires scope: updates:read
// @Tags    TUF
// @Produce json
// @Success 200
// @Router  /tuf/root.json [get]
func (h handlers) tufRootLatest(c echo.Context) error {
	return h.marshallTufRoot(c, 0)
}

// @Summary Returns a specific version of the TUF root metadata
// @Description Requires scope: updates:read
// @Tags    TUF
// @Produce json
// @Success 200
// @Param   version path string true "Root metadata file name, e.g. 3.root.json"
// @Router  /tuf/{version}.root.json [get]
func (h handlers) tufRootVersion(c echo.Context) error {
	name := c.Param("version")
	digits, found := strings.CutSuffix(name, rootJsonSuffix)
	if !found {
		return EchoError(c, nil, http.StatusNotFound, "not found")
	}
	version, err := strconv.Atoi(digits)
	if err != nil || version < 1 {
		return EchoError(c, err, http.StatusNotFound, "invalid root metadata version")
	}
	return h.marshallTufRoot(c, version)
}

func (h handlers) marshallTufRoot(c echo.Context, version int) error {
	data, err := h.storage.GetTufRoot(version)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return EchoError(c, err, http.StatusNotFound, "root metadata not found")
		}
		return EchoError(c, err, http.StatusInternalServerError, "failed to read root metadata")
	}
	return c.JSONBlob(http.StatusOK, data)
}
