// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"fmt"

	"github.com/foundriesio/dg-satellite/server/ui/api"
	"github.com/labstack/echo/v4"
)

func (h handlers) configsList(c echo.Context) error {
	var (
		configs api.ConfigFileSet
		groups  []string
	)
	if err := getJson(c.Request().Context(), "/v1/configs/factory", &configs); err != nil {
		return h.handleUnexpected(c, err)
	}
	if err := getJson(c.Request().Context(), "/v1/known-labels/device-groups", &groups); err != nil {
		return h.handleUnexpected(c, err)
	}

	ctx := struct {
		baseCtx
		Configs api.ConfigFileSet
		Groups  []string
	}{
		baseCtx: h.baseCtx(c, "Global Configs", "configs"),
		Configs: configs,
		Groups:  groups,
	}
	return h.templates.ExecuteTemplate(c.Response(), "configs_list.html", ctx)
}

func (h handlers) configsGroupItem(c echo.Context) error {
	var configs api.ConfigFileSet
	group := c.Param("name")
	if err := getJson(c.Request().Context(), "/v1/configs/group/"+group, &configs); err != nil {
		return h.handleUnexpected(c, err)
	}
	ctx := struct {
		baseCtx
		Configs api.ConfigFileSet
	}{
		baseCtx: h.baseCtx(c, fmt.Sprintf("Group \"%s\" Configs", group), "configs"),
		Configs: configs,
	}
	return h.templates.ExecuteTemplate(c.Response(), "configs_item.html", ctx)

}

func (h handlers) configsDeviceItem(c echo.Context) error {
	uuid := c.Param("uuid")
	var device api.Device
	if err := getJson(c.Request().Context(), "/v1/devices/"+uuid, &device); err != nil {
		return h.handleUnexpected(c, err)
	}
	var configs api.ConfigFileSet
	if err := getJson(c.Request().Context(), "/v1/configs/device/"+uuid, &configs); err != nil {
		return h.handleUnexpected(c, err)
	}
	ctx := struct {
		baseCtx
		Configs api.ConfigFileSet
	}{
		baseCtx: h.baseCtx(c, fmt.Sprintf("Device \"%s\" Configs", uuid), "devices"),
		Configs: configs,
	}
	return h.templates.ExecuteTemplate(c.Response(), "configs_item.html", ctx)

}

func (h handlers) configsDeviceItemApplied(c echo.Context) error {
	uuid := c.Param("uuid")
	var applied api.AppliedConfigs
	if err := getJson(c.Request().Context(), "/v1/configs/device/"+uuid+"/applied", &applied); err != nil {
		return h.handleUnexpected(c, err)
	}

	ctx := struct {
		baseCtx
		Configs api.AppliedConfigs
	}{
		baseCtx: h.baseCtx(c, fmt.Sprintf("Device \"%s\" Applied Config", uuid), "devices"),
		Configs: applied,
	}
	return h.templates.ExecuteTemplate(c.Response(), "applied_configs_item.html", ctx) // defined inside configs_item.html
}
