// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package gateway

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	storage "github.com/foundriesio/update-server/storage/gateway"
)

// @Summary Get the current TUF timestamp metadata
// @Produce json
// @Success 200
// @Router  /repo/timestamp.json [get]
func (h handlers) metaTimestamp(c echo.Context) error {
	return h.metaHandler(c, "timestamp", storage.TufTimestampFile)
}

// @Summary Get the current TUF snapshot metadata
// @Produce json
// @Success 200
// @Router  /repo/snapshot.json [get]
func (h handlers) metaSnapshot(c echo.Context) error {
	return h.metaHandler(c, "snapshot", storage.TufSnapshotFile)
}

// @Summary Get the current TUF targets metadata
// @Produce json
// @Success 200
// @Router  /repo/targets.json [get]
func (h handlers) metaTargets(c echo.Context) error {
	return h.metaHandler(c, "targets", storage.TufTargetsFile)
}

// @Summary Get the current TUF root metadata
// @Produce json
// @Param   version path int true "Root metadata version"
// @Success 200
// @Router  /repo/{version}.root.json [get]
func (h handlers) metaRoot(c echo.Context) error {
	version, err := readRootVersion(c)
	if err != nil {
		return err
	}
	file := fmt.Sprintf("%d.%s", version, storage.TufRootFile)
	return h.metaHandler(c, "root", file)
}

func (handlers) metaHandler(c echo.Context, role, file string) error {
	req := c.Request()
	ctx := req.Context()
	tag, err := readTagHeader(c)
	if err != nil {
		return err
	}
	log := CtxGetLog(ctx).With("tag", tag).With("role", role)
	c.SetRequest(req.WithContext(CtxWithLog(ctx, log)))

	d := CtxGetDevice(ctx)
	if content, err := d.GetTufMeta(tag, file); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return EchoError(c, err, http.StatusNotFound, "Not found TUF role")
		} else {
			return EchoError(c, err, http.StatusInternalServerError, "Failed to fetch TUF role")
		}
	} else {
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
		c.Response().Header().Set("x-ats-role-checksum", hash)
		return c.String(http.StatusOK, content)
	}
}

func readTagHeader(c echo.Context) (tag string, err error) {
	if tag = c.Request().Header.Get("x-ats-tags"); len(tag) == 0 {
		err = errors.New("device x-ats-tags header not set")
		err = EchoError(c, err, http.StatusNotFound, "Device sent no tag")
	}
	return
}

func readRootVersion(c echo.Context) (version int, err error) {
	if param := c.Param("root"); !strings.HasSuffix(param, ".root.json") {
		err = fmt.Errorf("versioned root URL does not end in .root.json: %s", param)
	} else if version, err = strconv.Atoi(param[:len(param)-10]); err != nil || version < 1 {
		err = fmt.Errorf("versioned root URL does not start with a positive integer: %s", param)
	}
	if err != nil {
		err = EchoError(c, err, http.StatusNotFound, "Not Found")
	}
	return
}
