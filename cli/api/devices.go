// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"fmt"
	"io"
	"net/url"
	"strconv"

	"github.com/foundriesio/dg-satellite/storage"
	models "github.com/foundriesio/dg-satellite/storage/api"
)

type DeviceListItem = models.DeviceListItem
type Device = models.Device
type DeviceUpdateEvent = models.DeviceUpdateEvent
type TargetTest = storage.TargetTest

type DeviceApi struct {
	api *Api
}

func (a *Api) Devices() DeviceApi {
	return DeviceApi{
		api: a,
	}
}

// ListPage fetches a single page of devices. It returns the devices,
// whether more pages are available, and the total number of pages.
func (d DeviceApi) ListPage(page int, limit int, sortBy string, labelFilters []string) ([]DeviceListItem, bool, int, error) {
	offset := (page - 1) * limit
	resource := fmt.Sprintf("/v1/devices?limit=%d&offset=%d", limit, offset)
	if sortBy != "" {
		resource += "&order-by=" + sortBy
	}
	for _, lf := range labelFilters {
		resource += "&label=" + url.QueryEscape(lf)
	}
	var devices []DeviceListItem
	headers, err := d.api.GetWithHeaders(resource, &devices)
	if err != nil {
		return nil, false, 0, err
	}
	linkHeader := headers.Get("Link")
	_, hasNext := ParseNextLink(linkHeader)
	totalPages := totalPagesFromLink(linkHeader, limit)
	return devices, hasNext, totalPages, nil
}

// totalPagesFromLink computes total pages from the rel="last" Link offset.
func totalPagesFromLink(linkHeader string, limit int) int {
	lastURL, ok := ParseLastLink(linkHeader)
	if !ok || limit <= 0 {
		return 0
	}
	parsed, err := url.Parse(lastURL)
	if err != nil {
		return 0
	}
	offsetStr := parsed.Query().Get("offset")
	if offsetStr == "" {
		return 0
	}
	lastOffset, err := strconv.Atoi(offsetStr)
	if err != nil {
		return 0
	}
	return (lastOffset / limit) + 1
}

func (d DeviceApi) Get(uuid string) (*Device, error) {
	var device Device
	if err := d.api.Get("/v1/devices/"+uuid, &device); err != nil {
		return nil, err
	}
	return &device, nil
}

func (d DeviceApi) Updates(uuid string) ([]string, error) {
	var updates []string
	return updates, d.api.Get(fmt.Sprintf("/v1/devices/%s/updates", uuid), &updates)
}

func (d DeviceApi) UpdateEvents(uuid, updateId string) ([]DeviceUpdateEvent, error) {
	var events []DeviceUpdateEvent
	return events, d.api.Get(fmt.Sprintf("/v1/devices/%s/updates/%s", uuid, updateId), &events)
}

func (d DeviceApi) Delete(uuid string) error {
	return d.api.Delete(fmt.Sprintf("/v1/devices/%s", uuid))
}

func (d *DeviceApi) Tests(uuid string) ([]TargetTest, error) {
	var tests []TargetTest
	return tests, d.api.Get(fmt.Sprintf("/v1/devices/%s/tests", uuid), &tests)
}

func (d *DeviceApi) Test(uuid, testId string) (*TargetTest, error) {
	var test TargetTest
	if err := d.api.Get(fmt.Sprintf("/v1/devices/%s/tests/%s", uuid, testId), &test); err != nil {
		return nil, err
	}
	return &test, nil
}

func (d *DeviceApi) TestArtifact(uuid, testId, artifact string) (io.ReadCloser, error) {
	return d.api.GetStream(fmt.Sprintf("/v1/devices/%s/tests/%s/%s", uuid, testId, artifact))
}
