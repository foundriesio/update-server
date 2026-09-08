// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/foundriesio/update-server/context"
	"github.com/foundriesio/update-server/server/ui/api"
	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/users"
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
	const pageSize = 50
	offset := (page - 1) * pageSize

	resource := fmt.Sprintf("/v1/devices?limit=%d&offset=%d", pageSize, offset)
	if sort != "" {
		resource += "&order-by=" + sort
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
		Devices    []api.DeviceListItem
		CanDelete  bool
		Page       int
		TotalPages int
		HasNext    bool
		HasPrev    bool
		Sort       string
	}{
		baseCtx:    h.baseCtx(c, "Devices", "devices"),
		Devices:    devices,
		CanDelete:  CtxGetSession(c.Request().Context()).User.AllowedScopes.Has(users.ScopeDevicesD),
		Page:       page,
		TotalPages: totalPages,
		HasNext:    hasNext,
		HasPrev:    page > 1,
		Sort:       sort,
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

	updates, err := fetchDeviceUpdates(c.Request().Context(), c.Param("uuid"))
	if err != nil {
		return h.handleUnexpected(c, err)
	}

	const overviewUpdatesLimit = 5
	recentUpdates := updates
	if len(recentUpdates) > overviewUpdatesLimit {
		recentUpdates = recentUpdates[:overviewUpdatesLimit]
	}

	ctx := struct {
		baseCtx
		Device        api.Device
		IpInfo        *ipInfo
		HwInfo        map[string]any
		RecentUpdates []string
		TotalUpdates  int
	}{
		baseCtx:       h.baseCtx(c, "Device - "+device.Uuid, "devices"),
		Device:        device,
		IpInfo:        infoPtr,
		HwInfo:        hw,
		RecentUpdates: recentUpdates,
		TotalUpdates:  len(updates),
	}
	return h.templates.ExecuteTemplate(c.Response(), "device.html", ctx)
}

// fetchDeviceUpdates fetches the reverse-chronological list of update
// correlation IDs for a device. Shared by devicesGet (capped preview) and
// devicesUpdatesGet (full history page).
func fetchDeviceUpdates(ctx context.Context, uuid string) ([]string, error) {
	var updates []string
	if err := getJson(ctx, "/v1/devices/"+uuid+"/updates", &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

func (h handlers) devicesUpdatesGet(c echo.Context) error {
	var device api.Device
	if err := getJson(c.Request().Context(), "/v1/devices/"+c.Param("uuid"), &device); err != nil {
		return h.handleUnexpected(c, err)
	}

	updates, err := fetchDeviceUpdates(c.Request().Context(), c.Param("uuid"))
	if err != nil {
		return h.handleUnexpected(c, err)
	}

	ctx := struct {
		baseCtx
		Device  api.Device
		Updates []string
	}{
		baseCtx: h.baseCtx(c, "Device - "+device.Uuid, "devices"),
		Device:  device,
		Updates: updates,
	}
	return h.templates.ExecuteTemplate(c.Response(), "device_updates.html", ctx)
}

// updateStep pairs a raw device update event with view-only fields the
// template needs but shouldn't compute itself: elapsed time since the
// previous step, and whether this specific step failed.
type updateStep struct {
	storage.DeviceUpdateEvent
	Failed  bool
	Elapsed string
	Time    string
}

// updateStepTimeFormat is the clock-only display format for a step's
// timestamp in the lifecycle stepper, e.g. "09:00:28".
const updateStepTimeFormat = "15:04:05"

// buildUpdateSteps enriches events (chronological, per the device-gateway
// API) with per-step elapsed time and failure state, and reports the
// update's overall pass/fail and total duration.
//
// failed is an all-or-nothing rollup: it is true if ANY event in the
// history reports Success == false, even if later steps succeeded. The
// UI has no notion of partial success, so one failed step marks the
// whole update as failed. Success == nil (no verdict reported for that
// event) never counts as a failure, at either the step or rollup level.
func buildUpdateSteps(events []storage.DeviceUpdateEvent) (steps []updateStep, duration string, failed bool) {
	steps = make([]updateStep, len(events))
	start := parseDeviceTime(events[0].DeviceTime)
	prev := start
	for i, ev := range events {
		t := parseDeviceTime(ev.DeviceTime)
		stepFailed := ev.Event.Success != nil && !*ev.Event.Success
		steps[i] = updateStep{
			DeviceUpdateEvent: ev,
			Failed:            stepFailed,
			Elapsed:           "+" + formatDuration(t.Sub(prev)),
			Time:              t.Format(updateStepTimeFormat),
		}
		failed = failed || stepFailed
		prev = t
	}
	end := parseDeviceTime(events[len(events)-1].DeviceTime)
	return steps, formatDuration(end.Sub(start)), failed
}

func parseDeviceTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return d.Round(time.Second).String()
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

	steps, duration, failed := buildUpdateSteps(events)

	ctx := struct {
		baseCtx
		Raw           string
		StartTime     string
		EndTime       string
		Duration      string
		Target        string
		DeviceUuid    string
		CorrelationId string
		Failed        bool
		Steps         []updateStep
	}{
		baseCtx:       h.baseCtx(c, "Update "+c.Param("update"), "devices"),
		Raw:           string(raw),
		Steps:         steps,
		Target:        events[0].Event.TargetName,
		StartTime:     events[0].DeviceTime,
		EndTime:       events[len(events)-1].DeviceTime,
		Duration:      duration,
		DeviceUuid:    c.Param("uuid"),
		CorrelationId: c.Param("update"),
		Failed:        failed,
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
