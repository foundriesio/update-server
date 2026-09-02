// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package gateway

import (
	"crypto/rand"
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
)

type ostreeDownloadUrl struct {
	DownloadUrl string `json:"download_url"`
	AccessToken string `json:"access_token"`
}

// @Summary Get the Ostree download URLs
// @Produce json
// @Success 200
// @Router  /ostree/download-urls [post]
func (h handlers) ostreeUrls(c echo.Context) error {
	d := CtxGetDevice(c.Request().Context())
	token := rand.Text()[:16]
	h.tokenCache.Set(token, d.Uuid, 0)
	return c.JSON(http.StatusOK, []ostreeDownloadUrl{
		{DownloadUrl: h.url + "/ostree", AccessToken: token},
	})
}

// @Summary Get the Ostree file contents
// @Produce octet-stream,plain,json
// @Param   path path string true "OSTree file path"
// @Success 200
// @Router  /ostree/{path} [get]
func (handlers) ostreeFileStream(c echo.Context) error {
	req := c.Request()
	ctx := req.Context()
	filePath := req.URL.Path[len("/ostree/"):]
	if filepath.Clean(filePath) != filePath {
		return c.String(http.StatusNotFound, "Not Found")
	}
	log := CtxGetLog(ctx).With("file", filePath)
	c.SetRequest(req.WithContext(CtxWithLog(ctx, log)))
	d := CtxGetDevice(c.Request().Context())
	if len(d.UpdateName) == 0 {
		err := errors.New("device has no updates configured")
		return EchoError(c, err, http.StatusBadRequest, err.Error())
	}
	c.Response().Header().Set(echo.HeaderContentType, ostreeContentType(filePath))
	return c.File(d.GetOstreeFilePath(filePath))
}

func ostreeContentType(path string) string {
	switch {
	case path == "config":
		return echo.MIMETextPlain
	case strings.HasPrefix(path, "delta-stats/"):
		return echo.MIMEApplicationJSON
	default:
		return echo.MIMEOctetStream
	}
}
