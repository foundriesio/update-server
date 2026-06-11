// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/foundriesio/update-server/server/ui/api"
	"github.com/foundriesio/update-server/storage/users"
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
		CanEdit bool
	}{
		baseCtx: h.baseCtx(c, "Global Configs", "configs"),
		Configs: configs,
		Groups:  groups,
		CanEdit: h.configsEditable(c),
	}
	return h.templates.ExecuteTemplate(c.Response(), "configs_list.html", ctx)
}

func (h handlers) configsGlobalHistory(c echo.Context) error {
	openIndex, err := echo.QueryParamOr[int](c, "open", -1)
	if err != nil {
		return h.handleUnexpected(c, err)
	}
	var history []api.ConfigFileSet
	uri := fmt.Sprintf("/v1/configs/factory/history?show-files=%t", openIndex >= 0)
	if err := getJson(c.Request().Context(), uri, &history); err != nil {
		return h.handleUnexpected(c, err)
	}
	ctx := struct {
		baseCtx
		History   []api.ConfigFileSet
		OpenIndex int
	}{
		baseCtx:   h.baseCtx(c, "Global Configs History", "configs"),
		History:   history,
		OpenIndex: openIndex,
	}
	return h.templates.ExecuteTemplate(c.Response(), "configs_history.html", ctx)
}

func (h handlers) configsGlobalPatch(c echo.Context) error {
	return h.configsPatchConfigFile(c, "/v1/configs/factory")
}

func (h handlers) configsGlobalDelete(c echo.Context) error {
	return h.configsDeleteConfigFile(c, "/v1/configs/factory")
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
		CanEdit bool
	}{
		baseCtx: h.baseCtx(c, fmt.Sprintf("Group \"%s\" Configs", group), "configs"),
		Configs: configs,
		CanEdit: h.configsEditable(c),
	}
	return h.templates.ExecuteTemplate(c.Response(), "configs_item.html", ctx)
}

func (h handlers) configsGroupItemHistory(c echo.Context) error {
	group := c.Param("name")
	openIndex, err := echo.QueryParamOr[int](c, "open", -1)
	if err != nil {
		return h.handleUnexpected(c, err)
	}
	var history []api.ConfigFileSet
	uri := fmt.Sprintf("/v1/configs/group/"+group+"/history?show-files=%t", openIndex >= 0)
	if err := getJson(c.Request().Context(), uri, &history); err != nil {
		return h.handleUnexpected(c, err)
	}
	ctx := struct {
		baseCtx
		History   []api.ConfigFileSet
		OpenIndex int
	}{
		baseCtx:   h.baseCtx(c, fmt.Sprintf("Group \"%s\" Configs History", group), "configs"),
		History:   history,
		OpenIndex: openIndex,
	}
	return h.templates.ExecuteTemplate(c.Response(), "configs_history.html", ctx)
}

func (h handlers) configsGroupItemPatch(c echo.Context) error {
	return h.configsPatchConfigFile(c, "/v1/configs/group/"+c.Param("name"))
}

func (h handlers) configsGroupItemDelete(c echo.Context) error {
	return h.configsDeleteConfigFile(c, "/v1/configs/group/"+c.Param("name"))
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
		CanEdit bool
	}{
		baseCtx: h.baseCtx(c, fmt.Sprintf("Device \"%s\" Configs", uuid), "devices"),
		Configs: configs,
		CanEdit: h.configsEditable(c),
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

func (h handlers) configsDeviceItemHistory(c echo.Context) error {
	uuid := c.Param("uuid")
	openIndex, err := echo.QueryParamOr[int](c, "open", -1)
	if err != nil {
		return h.handleUnexpected(c, err)
	}
	var history []api.ConfigFileSet
	uri := fmt.Sprintf("/v1/configs/device/"+uuid+"/history?show-files=%t", openIndex >= 0)
	if err := getJson(c.Request().Context(), uri, &history); err != nil {
		return h.handleUnexpected(c, err)
	}
	ctx := struct {
		baseCtx
		History   []api.ConfigFileSet
		OpenIndex int
	}{
		baseCtx:   h.baseCtx(c, fmt.Sprintf("Device \"%s\" Configs History", uuid), "devices"),
		History:   history,
		OpenIndex: openIndex,
	}
	return h.templates.ExecuteTemplate(c.Response(), "configs_history.html", ctx)
}

func (h handlers) configsDeviceItemPatch(c echo.Context) error {
	return h.configsPatchConfigFile(c, "/v1/configs/device/"+c.Param("uuid"))
}

func (h handlers) configsDeviceItemDelete(c echo.Context) error {
	return h.configsDeleteConfigFile(c, "/v1/configs/device/"+c.Param("uuid"))
}

func (h handlers) configsEditable(c echo.Context) bool {
	return CtxGetSession(c.Request().Context()).User.AllowedScopes.Has(users.ScopeDevicesRU | users.ScopeUpdatesRU)
}

func (h handlers) configsPatchConfigFile(c echo.Context, apiUrl string) error {
	ctx := c.Request().Context()
	var patch struct {
		Reason    string
		FileName  string
		OnChanged string
		Value     string
	}
	if err := c.Bind(&patch); err != nil {
		return EchoError(c, err, http.StatusBadRequest, "Could not parse request")
	} else if len(patch.FileName) == 0 {
		return EchoError(c, err, http.StatusBadRequest, "File name is mandatory")
	}
	var configs api.ConfigFileSet
	if err := getJson(ctx, apiUrl, &configs); err != nil {
		return h.handleUnexpected(c, err)
	}
	configs.Reason = patch.Reason
	if configs.Files == nil {
		configs.Files = make(map[string]api.ConfigFile)
	}
	configs.Files[patch.FileName] = api.ConfigFile{
		// Split by newline and skip white-space only lines.
		OnChanged:   strings.FieldsFunc(patch.OnChanged, func(r rune) bool { return r == '\n' }),
		Unencrypted: true,
		Value:       patch.Value,
	}
	if err := putJson(ctx, apiUrl, configs, nil); err != nil {
		return h.handleUnexpected(c, err)
	}
	return c.NoContent(http.StatusOK)
}

func (h handlers) configsDeleteConfigFile(c echo.Context, apiUrl string) error {
	ctx := c.Request().Context()
	var patch struct {
		FileName string
		Reason   string
	}
	if err := c.Bind(&patch); err != nil {
		return EchoError(c, err, http.StatusBadRequest, "Could not parse request")
	}
	var configs api.ConfigFileSet
	if err := getJson(ctx, apiUrl, &configs); err != nil {
		return h.handleUnexpected(c, err)
	}
	if configs.Files != nil {
		configs.Reason = patch.Reason
		delete(configs.Files, patch.FileName)
		if err := putJson(ctx, apiUrl, configs, nil); err != nil {
			return h.handleUnexpected(c, err)
		}
	}
	return c.NoContent(http.StatusOK)
}
