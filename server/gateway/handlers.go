// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package gateway

import (
	"time"

	cache "github.com/go-pkgz/expirable-cache/v3"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/foundriesio/dg-satellite/server"
	storage "github.com/foundriesio/dg-satellite/storage/gateway"
)

type handlers struct {
	url     string
	storage *storage.Storage

	tokenCache cache.Cache[string, string]
}

var (
	EchoError     = server.EchoError
	ReadBody      = server.ReadBody
	ReadJsonBody  = server.ReadJsonBody
	ParseJsonBody = server.ParseJsonBody
)

func RegisterHandlers(e *echo.Echo, storage *storage.Storage, url string) {
	cache := cache.NewCache[string, string]().WithMaxKeys(10000).WithTTL(time.Hour).WithLRU()
	h := handlers{storage: storage, url: url, tokenCache: cache}

	mtls := e.Group("/")
	mtls.Use(
		h.authDevice,
		middleware.BodyLimitWithConfig(middleware.BodyLimitConfig{ // After TLS authentication but before we read headers.
			Skipper: func(c echo.Context) bool {
				return c.Path() == "/tests/:testid/:path" // testArtifact can take large uploads
			},
			Limit: "100K",
		}),
		h.checkinDevice,
	)

	mtls.POST("apps-states", h.appsStatesInfo)
	mtls.POST("app-proxy-url", h.appsProxyUrl)
	mtls.GET("config", h.configGet)
	mtls.GET("device", h.deviceGet)
	mtls.POST("events", h.eventsUpload)
	mtls.POST("ostree/download-urls", h.ostreeUrls)
	mtls.GET("ostree/*", h.ostreeFileStream)
	mtls.GET("repo/timestamp.json", h.metaTimestamp)
	mtls.GET("repo/snapshot.json", h.metaSnapshot)
	mtls.GET("repo/targets.json", h.metaTargets)
	mtls.GET("repo/:root", h.metaRoot)
	mtls.PUT("system_info", h.hardwareInfo)
	mtls.PUT("system_info/config", h.akTomlInfo)
	mtls.PUT("system_info/network", h.networkInfo)
	mtls.POST("tests", h.testCreate)
	mtls.PUT("tests/:testid", h.testComplete)
	mtls.PUT("tests/:testid/:path", h.testArtifact)

	registry := e.Group("registry/v2")
	registry.Use(h.authToken)
	registry.HEAD("/*", h.blobHead)
	registry.GET("/*", h.blobGet)
}
