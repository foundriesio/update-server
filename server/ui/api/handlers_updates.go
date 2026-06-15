// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	storage "github.com/foundriesio/update-server/storage/api"
)

type UpdateTufResp map[string]map[string]any

// @Summary Create an update from a tar or tar+gz stream
// @Description Requires scope: updates:read-update
// @Tags    Updates
// @Accept  application/x-tar,application/gzip
// @Success 201
// @Param   tag path string true "Update tag"
// @Param   update path string true "Update name"
// @Param   hardware-id query string true "Hardware ID for TUF target (e.g. intel-corei7-64)"
// @Param   version query int true "Version number for TUF target"
// @Param   name query string false "Target name (defaults to ostree branch name)"
// @Param   ostree-hash query string false "OSTree hash (defaults to hash from ostree_repo/refs/heads/)"
// @Router  /updates/{tag}/{update} [post]
func (h handlers) updateCreate(c echo.Context) error {
	tag := c.Param("tag")
	update := c.Param("update")
	user := CtxGetUser(c.Request().Context())

	hardwareID := c.QueryParam("hardware-id")
	if hardwareID == "" {
		return EchoError(c, nil, http.StatusBadRequest, "missing required query parameter: hardware-id")
	}
	versionStr := c.QueryParam("version")
	if versionStr == "" {
		return EchoError(c, nil, http.StatusBadRequest, "missing required query parameter: version")
	}
	versionInt, err := strconv.Atoi(versionStr)
	if err != nil {
		return EchoError(c, err, http.StatusBadRequest, fmt.Sprintf("invalid version %q: must be an integer", versionStr))
	}

	params := &storage.TufTargetParams{
		HardwareID: hardwareID,
		Version:    strconv.Itoa(versionInt),
		Name:       c.QueryParam("name"),
		OstreeHash: c.QueryParam("ostree-hash"),
		Tag:        tag,
		BaseURI:    h.baseURI,
	}

	payload := c.Request().Body
	defer payload.Close() //nolint:errcheck

	if err := h.storage.CreateUpdate(tag, update, user.Username, params, payload); err != nil {
		if errors.Is(err, storage.ErrInvalidUpdate) {
			return EchoError(c, err, http.StatusBadRequest, err.Error())
		}
		return EchoError(c, err, http.StatusInternalServerError, "failed to create update")
	}

	return c.NoContent(http.StatusCreated)
}

// @Summary Returns the TUF metadata for the update
// @Description Requires scope: updates:read or updates:read-update
// @Tags    Updates
// @Produce json
// @Success 200 {object} UpdateTufResp
// @Param   tag path string true "Update tag"
// @Param   update path string true "Update name"
// @Router  /updates/{tag}/{update}/tuf [get]
func (h handlers) updateGetTuf(c echo.Context) error {
	tag := c.Param("tag")
	update := c.Param("update")

	metas, err := h.storage.GetUpdateTufMetadata(tag, update)
	if err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "failed to get update TUF metadata")
	}

	return c.JSON(http.StatusOK, metas)
}
