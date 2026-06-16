// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"errors"
	"net/http"

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
// @Router  /updates/{tag}/{update} [post]
func (h handlers) updateCreate(c echo.Context) error {
	tag := c.Param("tag")
	update := c.Param("update")

	payload := c.Request().Body
	defer payload.Close() //nolint:errcheck

	if err := h.storage.CreateUpdate(tag, update, payload); err != nil {
		switch {
		case errors.Is(err, storage.ErrInvalidUpdate):
			return EchoError(c, err, http.StatusBadRequest, err.Error())
		case errors.Is(err, storage.ErrDbConstraintUnique):
			return EchoError(c, err, http.StatusConflict, "Update with this name and tag already exists")
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
