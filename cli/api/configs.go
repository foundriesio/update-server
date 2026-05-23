// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"io"

	"github.com/foundriesio/dg-satellite/storage"
	models "github.com/foundriesio/dg-satellite/storage/api"
)

type (
	ConfigFile     = models.ConfigFile
	ConfigFileSet  = models.ConfigFileSet
	AppliedConfigs = storage.AppliedConfigs
)

type ConfigsApi struct {
	api *Api
}

type SpecificConfigsApi struct {
	api *Api
	uri string
}

type DeviceConfigsApi struct{ SpecificConfigsApi }

func (a *Api) Configs() ConfigsApi {
	return ConfigsApi{api: a}
}

func (a ConfigsApi) Factory() SpecificConfigsApi {
	return SpecificConfigsApi{api: a.api, uri: "/v1/configs/factory"}
}

func (a ConfigsApi) Group(name string) SpecificConfigsApi {
	return SpecificConfigsApi{api: a.api, uri: "/v1/configs/group/" + name}
}

func (a ConfigsApi) Device(uuid string) DeviceConfigsApi {
	return DeviceConfigsApi{SpecificConfigsApi: SpecificConfigsApi{api: a.api, uri: "/v1/configs/device/" + uuid}}
}

func (a ConfigsApi) ListGroups() (names []string, err error) {
	err = a.api.Get("/v1/known-labels/device-groups", &names)
	return
}

func (a ConfigsApi) Upload(r io.Reader, opts ...HttpOption) (err error) {
	_, err = a.api.Put("/v1/configs", r, opts...)
	return
}

func (a SpecificConfigsApi) Get() (res ConfigFileSet, err error) {
	err = a.api.Get(a.uri, &res)
	return
}

func (a SpecificConfigsApi) Put(configs ConfigFileSet) (err error) {
	_, err = a.api.Put(a.uri, configs)
	return
}

func (a DeviceConfigsApi) GetApplied() (res AppliedConfigs, err error) {
	err = a.api.Get(a.uri+"/applied", &res)
	return
}
