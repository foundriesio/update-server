// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/foundriesio/dg-satellite/auth"
	"github.com/foundriesio/dg-satellite/server"
	storage "github.com/foundriesio/dg-satellite/storage/api"
	"github.com/foundriesio/dg-satellite/storage/users"
)

type handlers struct {
	storage *storage.Storage
}

var EchoError = server.EchoError

func RegisterHandlers(e *echo.Echo, storage *storage.Storage, a auth.Provider) {
	h := handlers{storage: storage}
	g := e.Group("/v1")
	g.Use(authUser(a))

	g.PUT("/configs", h.configsUpload, requireScope(users.ScopeDevicesRU|users.ScopeUpdatesRU),
		gzipContentTypeAsContentEncoding, middleware.Decompress())
	g.GET("/configs/factory", h.factoryConfigsGet, requireScope(users.ScopeDevicesR))
	g.PUT("/configs/factory", h.factoryConfigsPut, requireScope(users.ScopeDevicesRU|users.ScopeUpdatesRU))
	g.GET("/configs/factory/history", h.factoryConfigsHistory, requireScope(users.ScopeDevicesR))
	g.GET("/configs/group/:name", h.groupConfigsGet, requireScope(users.ScopeDevicesR))
	g.PUT("/configs/group/:name", h.groupConfigsPut, requireScope(users.ScopeDevicesRU|users.ScopeUpdatesRU))
	g.GET("/configs/group/:name/history", h.groupConfigsHistory, requireScope(users.ScopeDevicesR))
	g.GET("/devices", h.deviceList, requireScope(users.ScopeDevicesR))
	g.GET("/devices/:uuid", h.deviceGet, requireScope(users.ScopeDevicesR))
	g.DELETE("/devices/:uuid", h.deviceDelete, requireScope(users.ScopeDevicesD))
	g.GET("/devices/:uuid/apps-states", h.deviceAppsStatesGet, requireScope(users.ScopeDevicesR))
	g.GET("/devices/:uuid/tests", h.deviceTestsList, requireScope(users.ScopeDevicesR))
	g.GET("/devices/:uuid/tests/:testid", h.deviceTestGet, requireScope(users.ScopeDevicesR))
	g.GET("/devices/:uuid/tests/:testid/:artifact", h.deviceTestArtifact, requireScope(users.ScopeDevicesR))
	g.GET("/devices/:uuid/updates", h.deviceUpdatesList, requireScope(users.ScopeDevicesR))
	g.GET("/devices/:uuid/updates/:id", h.deviceUpdatesGet, requireScope(users.ScopeDevicesR))
	g.PATCH("/devices/:uuid/labels", h.deviceLabelsPatch, requireScope(users.ScopeDevicesRU))
	g.PUT("/devices/:uuid/labels", h.deviceLabelsPut, requireScope(users.ScopeDevicesRU))
	g.GET("/known-labels/devices", h.deviceKnownLabelsGet, requireScope(users.ScopeDevicesR))
	g.GET("/known-labels/device-groups", h.deviceKnownGroupsGet, requireScope(users.ScopeDevicesR))
	// In updates APIs :prod path element can be either "prod" or "ci".
	upd := g.Group("/updates/:prod")
	upd.Use(validateUpdateParams)
	upd.GET("", h.updateList, requireScope(users.ScopeUpdatesR))
	upd.GET("/:tag", h.updateList, requireScope(users.ScopeUpdatesR))
	// TODO: What data would we want to show for an update?
	// upd.GET("/:tag/:update", h.updateGet, requireScope(users.ScopeDevicesR))
	upd.POST("/:tag/:update", h.updateCreate, requireScope(users.ScopeUpdatesRU),
		gzipContentTypeAsContentEncoding, middleware.Decompress())
	upd.GET("/:tag/:update/tuf", h.updateGetTuf, requireScope(users.ScopeUpdatesR))
	upd.GET("/:tag/:update/rollouts", h.rolloutList, requireScope(users.ScopeUpdatesR))
	upd.GET("/:tag/:update/rollouts/:rollout", h.rolloutGet, requireScope(users.ScopeUpdatesR))
	upd.PUT("/:tag/:update/rollouts/:rollout", h.rolloutPut, requireScope(users.ScopeUpdatesRU))
	upd.GET("/:tag/:update/rollouts/:rollout/tail", h.rolloutTail, requireScope(users.ScopeUpdatesR))
	upd.GET("/:tag/:update/tail", h.updateTail, requireScope(users.ScopeUpdatesR))
}
