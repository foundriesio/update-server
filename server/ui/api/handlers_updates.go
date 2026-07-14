// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

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
// @Param   version query int false "Override the target version (AppVersion)"
// @Param   name query string false "Override the target name"
// @Param   ostree-hash query string false "Override the ostree hash"
// @Param   apps query string false "Override docker compose apps as name=sha256[,name=sha256]"
// @Router  /updates/{tag}/{update} [post]
func (h handlers) updateCreate(c echo.Context) error {
	tag := c.Param("tag")
	update := c.Param("update")
	user := CtxGetUser(c.Request().Context())

	opts := storage.TargetOptions{
		Name:       c.QueryParam("name"),
		OstreeHash: c.QueryParam("ostree-hash"),
		Apps:       parseAppsParam(c.QueryParams()["apps"]),
		BaseUrl:    c.Request().Host,
	}
	if v := c.QueryParam("version"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return EchoError(c, err, http.StatusBadRequest, "invalid version parameter")
		}
		opts.AppVersion = n
	}

	payload := c.Request().Body
	defer payload.Close() //nolint:errcheck

	if err := h.storage.CreateUpdate(tag, update, user.Username, opts, payload); err != nil {
		switch {
		case errors.Is(err, storage.ErrInvalidUpdate):
			return EchoError(c, err, http.StatusBadRequest, err.Error())
		case errors.Is(err, storage.ErrDbConstraintUnique):
			return EchoError(c, err, http.StatusConflict, "Update with this name and tag already exists")
		}
		return EchoError(c, err, http.StatusInternalServerError, err.Error())
	}

	return c.NoContent(http.StatusCreated)
}

// parseAppsParam parses repeated "apps" query values of the form
// "name=sha256" (comma separated) into a map of app name to sha256.
func parseAppsParam(values []string) map[string]string {
	apps := make(map[string]string)
	for _, v := range values {
		for _, pair := range strings.Split(v, ",") {
			name, hash, ok := strings.Cut(pair, "=")
			if !ok {
				name, hash, ok = strings.Cut(pair, ":")
			}
			if !ok {
				continue
			}
			if name = strings.TrimSpace(name); name != "" {
				apps[name] = strings.TrimSpace(hash)
			}
		}
	}
	if len(apps) == 0 {
		return nil
	}
	return apps
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
