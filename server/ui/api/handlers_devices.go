// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/labstack/echo/v4"

	storage "github.com/foundriesio/dg-satellite/storage/api"
)

type (
	Device            = storage.Device
	DeviceListItem    = storage.DeviceListItem
	DeviceListOpts    = storage.DeviceListOpts
	DeviceUpdateEvent = storage.DeviceUpdateEvent
)

type AppsStatesResp struct {
	AppsStates []storage.AppsStates `json:"apps_states"`
}

type LabelsReq struct {
	Upserts map[string]string
	Deletes []string
}

type LabelsPutReq map[string]*string

// @Summary List devices
// @Description Requires scope: devices:read or devices:read-update
// @Tags    Devices
// @Param _ query DeviceListOpts false "Sorting options"
// @Param label query []string false "Label filters in the format key.comparison.value (e.g. env.eq.production). Comparison operators: eq, ne, contains, ncontains. Multiple filters are ANDed together."
// @Accept  json
// @Produce json
// @Success 200 {array} DeviceListItem
// @Header  200 {string} Link "Pagination links (first, next, last)"
// @Router  /devices [get]
func (h *handlers) deviceList(c echo.Context) error {
	opts := storage.DeviceListOpts{
		OrderBy: storage.OrderByDeviceNameAsc,
		Limit:   1000,
		Offset:  0,
	}
	if err := c.Bind(&opts); err != nil {
		return EchoError(c, err, http.StatusBadRequest, "Failed to parse list options")
	}

	if filters, err := parseLabelFilters(c.QueryParams()["label"]); err != nil {
		return EchoError(c, err, http.StatusBadRequest, err.Error())
	} else {
		opts.LabelFilters = filters
	}

	devices, total, err := h.storage.DevicesList(opts)
	if err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Unexpected error listing devices")
	}

	setPaginationHeaders(c, opts, total)
	return c.JSON(http.StatusOK, devices)
}

// parseLabelFilters parses query parameters of the form "key.comparison.value"
// into LabelFilter structs. The value may contain dots.
func parseLabelFilters(params []string) ([]storage.LabelFilter, error) {
	if len(params) == 0 {
		return nil, nil
	}
	filters := make([]storage.LabelFilter, 0, len(params))
	for _, p := range params {
		// Split into at most 3 parts: key, comparison, value (value may contain dots)
		parts := strings.SplitN(p, ".", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("invalid label filter %q: expected format key.comparison.value", p)
		}
		f := storage.LabelFilter{
			Label:      parts[0],
			Comparison: storage.LabelComparison(parts[1]),
			Value:      parts[2],
		}
		if err := f.Validate(); err != nil {
			return nil, err
		}
		filters = append(filters, f)
	}
	return filters, nil
}

func setPaginationHeaders(c echo.Context, opts storage.DeviceListOpts, total int) {
	if opts.Limit <= 0 {
		return
	}

	basePath := c.Request().URL.Path
	orderBy := string(opts.OrderBy)

	buildURL := func(offset int) string {
		return fmt.Sprintf("%s?offset=%d&limit=%d&order-by=%s", basePath, offset, opts.Limit, orderBy)
	}

	var links []string

	// first
	links = append(links, fmt.Sprintf("<%s>; rel=\"first\"", buildURL(0)))

	// next (only if there are more results)
	nextOffset := opts.Offset + opts.Limit
	if nextOffset < total {
		links = append(links, fmt.Sprintf("<%s>; rel=\"next\"", buildURL(nextOffset)))
	}

	// last
	lastOffset := 0
	if total > opts.Limit {
		lastOffset = ((total - 1) / opts.Limit) * opts.Limit
	}
	links = append(links, fmt.Sprintf("<%s>; rel=\"last\"", buildURL(lastOffset)))

	c.Response().Header().Set("Link", strings.Join(links, ", "))
}

// @Summary Get a device by its UUID
// @Description Requires scope: devices:read or devices:read-update
// @Tags    Devices
// @Produce json
// @Success 200 {object} Device
// @Param   uuid path string true "Device UUID"
// @Router  /devices/{uuid} [get]
func (h *handlers) deviceGet(c echo.Context) error {
	return h.handleDevice(c, func(device *Device) error {
		return c.JSON(http.StatusOK, device)
	})
}

// @Summary Delete a device
// @Description Requires scope: devices:delete
// @Tags    Devices
// @Produce json
// @Success 204
// @Param   uuid path string true "Device UUID"
// @Router  /devices/{uuid} [delete]
func (h *handlers) deviceDelete(c echo.Context) error {
	return h.handleDevice(c, func(device *Device) error {
		if err := device.Delete(); err != nil {
			return EchoError(c, err, http.StatusInternalServerError, "Failed to delete device")
		}
		return c.NoContent(http.StatusNoContent)
	})
}

// @Summary Get a list of updates for a device
// @Description Requires scope: devices:read or devices:read-update
// @Tags    Devices
// @Produce json
// @Success 200 {array} string
// @Param   uuid path string true "Device UUID"
// @Router  /devices/{uuid}/updates [get]
func (h *handlers) deviceUpdatesList(c echo.Context) error {
	return h.handleDevice(c, func(device *Device) error {
		updates, err := device.Updates()
		if err != nil {
			return EchoError(c, err, http.StatusInternalServerError, "Failed to lookup device updates")
		}
		return c.JSON(http.StatusOK, updates)
	})
}

// @Summary Get details of update events for a devices
// @Description Requires scope: devices:read or devices:read-update
// @Tags    Devices
// @Produce json
// @Success 200 {array} DeviceUpdateEvent
// @Param   uuid path string true "Device UUID"
// @Param   id path string true "Update ID"
// @Router  /devices/{uuid}/updates/{id} [get]
func (h *handlers) deviceUpdatesGet(c echo.Context) error {
	return h.handleDevice(c, func(device *Device) error {
		updateId := c.Param("id")
		if !storage.ValidCorrelationId(updateId) {
			return c.NoContent(http.StatusNotFound)
		}
		events, err := device.Events(updateId)
		if err != nil {
			return EchoError(c, err, http.StatusInternalServerError, "Failed to lookup device update events")
		}
		if len(events) == 0 {
			return c.NoContent(http.StatusNotFound)
		}
		return c.JSON(http.StatusOK, events)
	})
}

// @Summary Get a list of Apps states reported by the device
// @Description Requires scope: devices:read or devices:read-update
// @Tags    Devices
// @Produce json
// @Success 200 {object} AppsStatesResp
// @Param   uuid path string true "Device UUID"
// @Router  /devices/{uuid}/apps-states [get]
func (h *handlers) deviceAppsStatesGet(c echo.Context) error {
	return h.handleDevice(c, func(device *Device) error {
		appsStates, err := device.AppsStates()
		if err != nil {
			return EchoError(c, err, http.StatusInternalServerError, "Failed to lookup device updates")
		}
		return c.JSON(http.StatusOK, AppsStatesResp{AppsStates: appsStates})
	})
}

// @Summary Get known device group names
// @Description Requires scope: devices:read or devices:read-update
// @Tags    Devices
// @Produce json
// @Success 200 {array} string
// @Router  /known-labels/device-groups [get]
func (h *handlers) deviceKnownGroupsGet(c echo.Context) error {
	if groups, err := h.storage.GetKnownDeviceGroupNames(); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to lookup known device groups")
	} else {
		return c.JSON(http.StatusOK, groups)
	}
}

var standardLabels = []string{"name", "group"}

// @Summary Get known device label names
// @Description Requires scope: devices:read or devices:read-update
// @Tags    Devices
// @Produce json
// @Success 200 {array} string
// @Router  /known-labels/devices [get]
func (h *handlers) deviceKnownLabelsGet(c echo.Context) error {
	if labels, err := h.storage.GetKnownDeviceLabelNames(); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to lookup known device labels")
	} else {
		// Make sure that standard labels are always present
		labels = slices.DeleteFunc(labels, func(s string) bool {
			return slices.Contains(standardLabels, s)
		})
		labels = append(standardLabels, labels...)
		return c.JSON(http.StatusOK, labels)
	}
}

// @Summary Patch device labels
// @Description Requires scope: devices:read-update
// @Tags    Devices
// @Accept json
// @Param data body LabelsReq true "Labels to upsert or delete"
// @Success 200
// @Param   uuid path string true "Device UUID"
// @Router  /devices/{uuid}/labels [patch]
func (h *handlers) deviceLabelsPatch(c echo.Context) error {
	return h.handleDevice(c, func(device *Device) error {
		var labelsReq LabelsReq
		if err := c.Bind(&labelsReq); err != nil {
			return EchoError(c, err, http.StatusBadRequest, "Bad JSON body")
		}
		if labels, err := parseLabels(labelsReq); err != nil {
			return EchoError(c, err, http.StatusBadRequest, err.Error())
		} else if err = h.storage.PatchDeviceLabels(labels, []string{device.Uuid}); err != nil {
			if storage.IsDbError(err, storage.ErrDbConstraintUnique) {
				return EchoError(c, err, http.StatusConflict, "A device with the same 'name' label value already exists")
			}
			return EchoError(c, err, http.StatusInternalServerError, "Failed to update device labels")
		}
		return c.NoContent(http.StatusOK)
	})
}

// @Summary Put device labels
// @Description Requires scope: devices:read-update
// @Tags    Devices
// @Accept json
// @Param data body LabelsPutReq true "Labels to set"
// @Success 200
// @Param   uuid path string true "Device UUID"
// @Router  /devices/{uuid}/labels [put]
func (h *handlers) deviceLabelsPut(c echo.Context) error {
	return h.handleDevice(c, func(device *Device) error {
		var labels LabelsPutReq
		if err := c.Bind(&labels); err != nil {
			return EchoError(c, err, http.StatusBadRequest, "Bad JSON body")
		}
		if err := validateLabels(labels); err != nil {
			return EchoError(c, err, http.StatusBadRequest, err.Error())
		}

		for k := range device.Labels {
			if _, ok := labels[k]; !ok {
				labels[k] = nil
			}
		}

		if err := h.storage.PatchDeviceLabels(labels, []string{device.Uuid}); err != nil {
			if storage.IsDbError(err, storage.ErrDbConstraintUnique) {
				return EchoError(c, err, http.StatusConflict, "A device with the same 'name' label value already exists")
			}
			return EchoError(c, err, http.StatusInternalServerError, "Failed to update device labels")
		}
		return c.NoContent(http.StatusOK)
	})
}

func (h *handlers) handleDevice(c echo.Context, next func(*Device) error) error {
	uuid := c.Param("uuid")
	if device, err := h.storage.DeviceGet(uuid); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to lookup device")
	} else if device == nil {
		return c.NoContent(http.StatusNotFound)
	} else {
		return next(device)
	}
}

const (
	// Together with a 2048 limit on total labels JSONB size,
	// these constraints allow at least 24 labels per device (realistic limit is around 60-70).
	maxLabelName  = 20
	maxLabelValue = 60
	// Label names are lowercase only; label values are case-sensitive.
	validLabelNameRegex  = `^[a-z0-9_\-\.]+$`
	validLabelValueRegex = `^[a-zA-Z0-9_\-\.]+$`
)

var (
	validateLabelName  = regexp.MustCompile(validLabelNameRegex).MatchString
	validateLabelValue = regexp.MustCompile(validLabelValueRegex).MatchString
)

func parseLabels(req LabelsReq) (map[string]*string, error) {
	if len(req.Upserts) == 0 && len(req.Deletes) == 0 {
		return nil, fmt.Errorf("at least one label change must be requested")
	}
	labels := make(map[string]*string, len(req.Upserts)+len(req.Deletes))
	for k, v := range req.Upserts {
		labels[k] = &v
	}
	for _, k := range req.Deletes {
		if _, ok := labels[k]; ok {
			return nil, fmt.Errorf("a label %s cannot be both updated and deleted at once", k)
		}
		labels[k] = nil
	}
	if err := validateLabels(labels); err != nil {
		return nil, err
	}
	return labels, nil
}

func validateLabels(labels map[string]*string) error {
	for k, v := range labels {
		switch {
		case len(k) > maxLabelName:
			return fmt.Errorf("label %s exceeds maximum label name limit %d", k, maxLabelName)
		case v != nil && len(*v) > maxLabelValue:
			return fmt.Errorf("label %s exceeds maximum label value limit %d", k, maxLabelValue)
		case !validateLabelName(k):
			return fmt.Errorf("label %s name must match a given regexp: %s", k, validLabelNameRegex)
		case v != nil && !validateLabelValue(*v):
			return fmt.Errorf("label %s value must match a given regexp: %s", k, validLabelValueRegex)
		}
	}
	return nil
}
