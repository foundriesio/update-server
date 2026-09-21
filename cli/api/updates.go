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
type UpdateSummary = models.UpdateSummary

type UpdatesApi struct {
	api *Api
}

func (a *Api) Updates() UpdatesApi {
	return UpdatesApi{api: a}
}

func (u UpdatesApi) List() ([]Update, error) {
	var updates []Update
	return updates, u.api.Get("/v1/updates", &updates)
}

func (u UpdatesApi) Get(updateName string) ([]string, error) {
	var rollouts []string
	endpoint := "/v1/updates/" + updateName + "/rollouts"
	return rollouts, u.api.Get(endpoint, &rollouts)
}

func (u UpdatesApi) GetSummary(updateName string) (UpdateSummary, error) {
	var summary UpdateSummary
	endpoint := "/v1/updates/" + updateName + "/summary"
	return summary, u.api.Get(endpoint, &summary)
}

func (u UpdatesApi) GetDevicesForStatus(updateName, status string) ([]string, error) {
	var devices []string
	endpoint := "/v1/updates/" + updateName + "/query?status=" + url.QueryEscape(status)
	return devices, u.api.Get(endpoint, &devices)
}

// UpdateTuf holds the TUF metadata for an update keyed by role file name
// (e.g. "root.json", "targets.json"). Each value is the parsed JSON of the
// corresponding metadata file.
type UpdateTuf = map[string]map[string]any

func (u UpdatesApi) GetTuf(updateName string) (UpdateTuf, error) {
	var tuf UpdateTuf
	endpoint := "/v1/updates/" + updateName + "/tuf"
	return tuf, u.api.Get(endpoint, &tuf)
}

func (u UpdatesApi) Tail(updateName string) (io.ReadCloser, error) {
	endpoint := "/v1/updates/" + updateName + "/tail"
	return u.api.GetStream(endpoint)
}

func (u UpdatesApi) GetRollout(updateName, rollout string) (Rollout, error) {
	var r Rollout
	endpoint := "/v1/updates/" + updateName + "/rollouts/" + rollout
	return r, u.api.Get(endpoint, &r)
}

func (u UpdatesApi) GetRolloutSummary(updateName, rollout string) (UpdateSummary, error) {
	var report UpdateSummary
	endpoint := "/v1/updates/" + updateName + "/rollouts/" + rollout + "/summary"
	return report, u.api.Get(endpoint, &report)
}

func (u UpdatesApi) GetDevicesForRolloutStatus(updateName, rollout, status string) ([]string, error) {
	var devices []string
	endpoint := "/v1/updates/" + updateName + "/rollouts/" + rollout + "/query?status=" + url.QueryEscape(status)
	return devices, u.api.Get(endpoint, &devices)
}

func (u UpdatesApi) CreateRollout(tag, updateName, rollout string, data Rollout) error {
	endpoint := "/v1/updates/" + tag + "/" + updateName + "/rollouts/" + rollout
	_, err := u.api.Put(endpoint, data)
	return err
}

func (u UpdatesApi) TailRollout(updateName, rollout string) (io.ReadCloser, error) {
	endpoint := "/v1/updates/" + updateName + "/rollouts/" + rollout + "/tail"
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

func (u UpdatesApi) Delete(updateName string) error {
	endpoint := "/v1/updates/" + updateName
	return u.api.Delete(endpoint)
}
