// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

func (h handlers) tufRootLatest(c echo.Context) error {
	return h.serveTufRoot(c, 0)
}

func (h handlers) tufRootByVersion(c echo.Context) error {
	versionParam := c.Param("version")
	// Accept both "N.root.json" and plain "N"
	versionParam = strings.TrimSuffix(versionParam, ".root.json")
	ver, err := strconv.Atoi(versionParam)
	if err != nil || ver < 1 {
		return EchoError(c, nil, http.StatusBadRequest, "invalid root version")
	}
	return h.serveTufRoot(c, ver)
}

func (h handlers) serveTufRoot(c echo.Context, version int) error {
	data, err := h.tuf.GetRootJSON(version)
	if err != nil {
		return EchoError(c, err, http.StatusNotFound, err.Error())
	}
	return c.JSONBlob(http.StatusOK, []byte(data))
}
