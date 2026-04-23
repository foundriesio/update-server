// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/foundriesio/dg-satellite/context"
	"github.com/foundriesio/dg-satellite/server/ui/api"
	"github.com/foundriesio/dg-satellite/storage"
	"github.com/foundriesio/dg-satellite/storage/users"
	"github.com/labstack/echo/v4"
)

func (h handlers) devicesList(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	sort := c.QueryParam("sort")
	if sort == "" {
		sort = "created-at-desc"
	}
	labelFilters := c.QueryParams()["label"]

	const pageSize = 50
	offset := (page - 1) * pageSize

	resource := fmt.Sprintf("/v1/devices?limit=%d&offset=%d", pageSize, offset)
	if sort != "" {
		resource += "&order-by=" + sort
	}
	for _, lf := range labelFilters {
		resource += "&label=" + lf
	}

	var devices []api.DeviceListItem
	headers, err := getJsonWithHeaders(c.Request().Context(), resource, &devices)
	if err != nil {
		return h.handleUnexpected(c, err)
	}

	hasNext := linkHasRel(headers.Get("Link"), "next")
	totalPages := linkTotalPages(headers.Get("Link"), pageSize)

	ctx := struct {
		baseCtx
		Devices      []api.DeviceListItem
		CanDelete    bool
		Page         int
		TotalPages   int
		HasNext      bool
		HasPrev      bool
		Sort         string
		LabelFilters []string
	}{
		baseCtx:      h.baseCtx(c, "Devices", "devices"),
		Devices:      devices,
		CanDelete:    CtxGetSession(c.Request().Context()).User.AllowedScopes.Has(users.ScopeDevicesD),
		Page:         page,
		TotalPages:   totalPages,
		HasNext:      hasNext,
		HasPrev:      page > 1,
		Sort:         sort,
		LabelFilters: labelFilters,
	}
	return h.templates.ExecuteTemplate(c.Response(), "devices_list.html", ctx)
}

// linkHasRel checks if a Link header contains a given rel value.
func linkHasRel(linkHeader, rel string) bool {
	target := fmt.Sprintf(`rel="%s"`, rel)
	for part := range strings.SplitSeq(linkHeader, ",") {
		if strings.Contains(strings.TrimSpace(part), target) {
			return true
		}
	}
	return false
}

// linkTotalPages extracts the last page's offset from the Link header to compute total pages.
func linkTotalPages(linkHeader string, pageSize int) int {
	for part := range strings.SplitSeq(linkHeader, ",") {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, `rel="last"`) {
			continue
		}
		// Extract URL between < and >
		start := strings.Index(part, "<")
		end := strings.Index(part, ">")
		if start < 0 || end < 0 || end <= start {
			return 1
		}
		url := part[start+1 : end]
		// Find offset parameter
		for param := range strings.SplitSeq(url[strings.Index(url, "?")+1:], "&") {
			if strings.HasPrefix(param, "offset=") {
				if offset, err := strconv.Atoi(param[len("offset="):]); err == nil && pageSize > 0 {
					return offset/pageSize + 1
				}
			}
		}
	}
	return 1
}

type ipInfo struct {
	Hostname string `json:"hostname"`
	IP       string `json:"local_ipv4"`
	Mac      string `json:"mac"`
}

func (h handlers) devicesGet(c echo.Context) error {
	var device api.Device
	if err := getJson(c.Request().Context(), "/v1/devices/"+c.Param("uuid"), &device); err != nil {
		return h.handleUnexpected(c, err)
	}

	var info ipInfo
	infoPtr := &info
	if err := json.Unmarshal([]byte(device.NetInfo), &info); err != nil {
		context.CtxGetLog(c.Request().Context()).Warn("failed to parse device netinfo", "err", err)
		infoPtr = nil
	}

	var hw map[string]any
	if err := json.Unmarshal([]byte(device.HwInfo), &hw); err != nil {
		context.CtxGetLog(c.Request().Context()).Warn("failed to parse device hardware info", "err", err)
	} else {
		indentBytes, err := json.MarshalIndent(hw, "", "  ")
		if err != nil {
			context.CtxGetLog(c.Request().Context()).Warn("failed to re-marshal device hardware info", "err", err)
		} else {
			device.HwInfo = string(indentBytes)
		}
	}

	var updates []string
	if err := getJson(c.Request().Context(), "/v1/devices/"+c.Param("uuid")+"/updates", &updates); err != nil {
		return h.handleUnexpected(c, err)
	}

	ctx := struct {
		baseCtx
		Device  api.Device
		IpInfo  *ipInfo
		HwInfo  map[string]any
		Updates []string
	}{
		baseCtx: h.baseCtx(c, "Device - "+device.Uuid, "devices"),
		Device:  device,
		IpInfo:  infoPtr,
		HwInfo:  hw,
		Updates: updates,
	}
	return h.templates.ExecuteTemplate(c.Response(), "device.html", ctx)
}

func (h handlers) devicesUpdateGet(c echo.Context) error {
	var events []storage.DeviceUpdateEvent
	if err := getJson(c.Request().Context(), "/v1/devices/"+c.Param("uuid")+"/updates/"+c.Param("update"), &events); err != nil {
		return h.handleUnexpected(c, err)
	}

	raw, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		return h.handleUnexpected(c, err)
	}

	ctx := struct {
		baseCtx
		Raw       string
		StartTime string
		EndTime   string
		Target    string
		Events    []storage.DeviceUpdateEvent
	}{
		baseCtx:   h.baseCtx(c, "Device - "+c.Param("uuid"), "update: "+c.Param("update")),
		Raw:       string(raw),
		Events:    events,
		Target:    events[0].Event.TargetName,
		StartTime: events[0].DeviceTime,
		EndTime:   events[len(events)-1].DeviceTime,
	}
	return h.templates.ExecuteTemplate(c.Response(), "device_update.html", ctx)
}

func (h handlers) devicesAppsStates(c echo.Context) error {
	type appState struct {
		AppsStates []storage.AppsStates `json:"apps_states"`
	}
	var states appState
	if err := getJson(c.Request().Context(), "/v1/devices/"+c.Param("uuid")+"/apps-states", &states); err != nil {
		return h.handleUnexpected(c, err)
	}

	ctx := struct {
		baseCtx
		Apps []storage.AppsStates
	}{
		baseCtx: h.baseCtx(c, "Device - "+c.Param("uuid")+" Apps States", "devices"),
		Apps:    states.AppsStates,
	}
	return h.templates.ExecuteTemplate(c.Response(), "device_apps_states.html", ctx)
}

func (h handlers) devicesTests(c echo.Context) error {
	var tests []storage.TargetTest
	if err := getJson(c.Request().Context(), "/v1/devices/"+c.Param("uuid")+"/tests", &tests); err != nil {
		return h.handleUnexpected(c, err)
	}

	ctx := struct {
		baseCtx
		DeviceUuid string
		Tests      []storage.TargetTest
	}{
		baseCtx:    h.baseCtx(c, "Device - "+c.Param("uuid")+" Tests", "devices"),
		DeviceUuid: c.Param("uuid"),
		Tests:      tests,
	}
	return h.templates.ExecuteTemplate(c.Response(), "device_tests.html", ctx)
}

func (h handlers) devicesTestGet(c echo.Context) error {
	var test storage.TargetTest
	if err := getJson(c.Request().Context(), "/v1/devices/"+c.Param("uuid")+"/tests/"+c.Param("testid"), &test); err != nil {
		return h.handleUnexpected(c, err)
	}

	ctx := struct {
		baseCtx
		DeviceUuid string
		Test       storage.TargetTest
	}{
		baseCtx:    h.baseCtx(c, "Device - "+c.Param("uuid")+" Test - "+test.Name, "devices"),
		DeviceUuid: c.Param("uuid"),
		Test:       test,
	}
	return h.templates.ExecuteTemplate(c.Response(), "device_test.html", ctx)
}

func (h handlers) devicesLabelsGet(c echo.Context) error {
	var device api.Device
	if err := getJson(c.Request().Context(), "/v1/devices/"+c.Param("uuid"), &device); err != nil {
		return h.handleUnexpected(c, err)
	}
	var knownLabels []string
	if err := getJson(c.Request().Context(), "/v1/known-labels/devices", &knownLabels); err != nil {
		return h.handleUnexpected(c, err)
	}
	var knownGroups []string
	if err := getJson(c.Request().Context(), "/v1/known-labels/device-groups", &knownGroups); err != nil {
		return h.handleUnexpected(c, err)
	}

	ctx := struct {
		baseCtx
		Device      api.Device
		KnownLabels []string
		KnownGroups []string
	}{
		baseCtx:     h.baseCtx(c, "Manage labels for - "+device.Uuid, "devices"),
		Device:      device,
		KnownLabels: knownLabels,
		KnownGroups: knownGroups,
	}
	return h.templates.ExecuteTemplate(c.Response(), "device_labels.html", ctx)
}
