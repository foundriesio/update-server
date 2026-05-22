// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"io"

	models "github.com/foundriesio/dg-satellite/storage/api"
)

type Rollout = models.Rollout

type UpdatesApi struct {
	api  *Api
	Type string
}

// Updates returns an UpdatesApi instance for either "ci" or "prod" updates.
func (a *Api) Updates(updateType string) UpdatesApi {
	return UpdatesApi{
		api:  a,
		Type: updateType,
	}
}

func (u UpdatesApi) List() (map[string][]string, error) {
	var updates map[string][]string
	return updates, u.api.Get("/v1/updates/"+u.Type, &updates)
}

func (u UpdatesApi) Get(tag, updateName string) ([]string, error) {
	var rollouts []string
	endpoint := "/v1/updates/" + u.Type + "/" + tag + "/" + updateName + "/rollouts"
	return rollouts, u.api.Get(endpoint, &rollouts)
}

func (u UpdatesApi) Tail(tag, updateName string) (io.ReadCloser, error) {
	endpoint := "/v1/updates/" + u.Type + "/" + tag + "/" + updateName + "/tail"
	return u.api.GetStream(endpoint)
}

func (u UpdatesApi) GetRollout(tag, updateName, rollout string) (Rollout, error) {
	var r Rollout
	endpoint := "/v1/updates/" + u.Type + "/" + tag + "/" + updateName + "/rollouts/" + rollout
	return r, u.api.Get(endpoint, &r)
}

func (u UpdatesApi) CreateRollout(tag, updateName, rollout string, data Rollout) error {
	endpoint := "/v1/updates/" + u.Type + "/" + tag + "/" + updateName + "/rollouts/" + rollout
	_, err := u.api.Put(endpoint, data)
	return err
}

func (u UpdatesApi) TailRollout(tag, updateName, rollout string) (io.ReadCloser, error) {
	endpoint := "/v1/updates/" + u.Type + "/" + tag + "/" + updateName + "/rollouts/" + rollout + "/tail"
	return u.api.GetStream(endpoint)
}

func (u UpdatesApi) CreateUpdate(tag, updateName string, body io.Reader) error {
	endpoint := "/v1/updates/" + u.Type + "/" + tag + "/" + updateName
	_, err := u.api.Post(endpoint, body, HttpHeader("Content-Type", "application/x-tar"), HttpHeader("Content-Encoding", "gzip"))
	return err
}
