// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"io"
	"net/url"
	"strconv"

	models "github.com/foundriesio/update-server/storage/api"
)

type Rollout = models.Rollout
type Update = models.Update

type UpdatesApi struct {
	api *Api
}

func (a *Api) Updates() UpdatesApi {
	return UpdatesApi{api: a}
}

func (u UpdatesApi) List() (map[string][]Update, error) {
	var updates map[string][]Update
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

// CreateUpdateOptions captures the optional TUF target overrides that can be
// supplied when creating an update.
type CreateUpdateOptions struct {
	Version    int               // Override the target version (AppVersion)
	HardwareId string            // Override the hardware id
	Name       string            // Override the target name
	OstreeHash string            // Override the ostree hash
	Apps       map[string]string // Override docker compose apps (name -> sha256)
}

func (o CreateUpdateOptions) query() string {
	values := url.Values{}
	if o.Version != 0 {
		values.Set("version", strconv.Itoa(o.Version))
	}
	if o.Name != "" {
		values.Set("name", o.Name)
	}
	if o.HardwareId != "" {
		values.Set("hardware-id", o.HardwareId)
	}
	if o.OstreeHash != "" {
		values.Set("ostree-hash", o.OstreeHash)
	}
	for name, hash := range o.Apps {
		values.Add("apps", name+"="+hash)
	}
	if len(values) == 0 {
		return ""
	}
	return "?" + values.Encode()
}

func (u UpdatesApi) CreateUpdate(tag, updateName string, opts CreateUpdateOptions, body io.Reader) error {
	endpoint := "/v1/updates/" + tag + "/" + updateName + opts.query()
	_, err := u.api.Post(endpoint, body, HttpHeader("Content-Type", "application/x-tar"), HttpHeader("Content-Encoding", "gzip"))
	return err
}
