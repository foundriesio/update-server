// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"io"

	models "github.com/foundriesio/update-server/storage/api"
)

type Rollout = models.Rollout

type UpdatesApi struct {
	api *Api
}

func (a *Api) Updates() UpdatesApi {
	return UpdatesApi{api: a}
}

func (u UpdatesApi) List() (map[string][]string, error) {
	var updates map[string][]string
	return updates, u.api.Get("/v1/updates", &updates)
}

func (u UpdatesApi) Get(tag, updateName string) ([]string, error) {
	var rollouts []string
	endpoint := "/v1/updates/" + tag + "/" + updateName + "/rollouts"
	return rollouts, u.api.Get(endpoint, &rollouts)
}

func (u UpdatesApi) Tail(tag, updateName string) (io.ReadCloser, error) {
	endpoint := "/v1/updates/" + tag + "/" + updateName + "/tail"
	return u.api.GetStream(endpoint)
}

func (u UpdatesApi) GetRollout(tag, updateName, rollout string) (Rollout, error) {
	var r Rollout
	endpoint := "/v1/updates/" + tag + "/" + updateName + "/rollouts/" + rollout
	return r, u.api.Get(endpoint, &r)
}

func (u UpdatesApi) CreateRollout(tag, updateName, rollout string, data Rollout) error {
	endpoint := "/v1/updates/" + tag + "/" + updateName + "/rollouts/" + rollout
	_, err := u.api.Put(endpoint, data)
	return err
}

func (u UpdatesApi) TailRollout(tag, updateName, rollout string) (io.ReadCloser, error) {
	endpoint := "/v1/updates/" + tag + "/" + updateName + "/rollouts/" + rollout + "/tail"
	return u.api.GetStream(endpoint)
}

func (u UpdatesApi) CreateUpdate(tag, updateName string, body io.Reader) error {
	endpoint := "/v1/updates/" + tag + "/" + updateName
	_, err := u.api.Post(endpoint, body, HttpHeader("Content-Type", "application/x-tar"), HttpHeader("Content-Encoding", "gzip"))
	return err
}
