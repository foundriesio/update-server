// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/foundriesio/update-server/server/ui/api"
	"github.com/labstack/echo/v4"
)

type latestTarget struct {
	Name    string
	Version string
	Sha256  string
	Apps    map[string]string // app name -> uri
}

func findLatestTarget(tuf api.UpdateTufResp) *latestTarget {
	targetsJson, ok := tuf["targets.json"]
	if !ok {
		return nil
	}
	signed, ok := targetsJson["signed"].(map[string]any)
	if !ok {
		return nil
	}
	targets, ok := signed["targets"].(map[string]any)
	if !ok {
		return nil
	}

	var latest *latestTarget
	latestVersion := -1

	for name, target := range targets {
		t, ok := target.(map[string]any)
		if !ok {
			continue
		}
		custom, ok := t["custom"].(map[string]any)
		if !ok {
			continue
		}
		versionStr, ok := custom["version"].(string)
		if !ok {
			continue
		}
		version, err := strconv.Atoi(versionStr)
		if err != nil {
			continue
		}
		if version > latestVersion {
			latestVersion = version

			sha256 := ""
			if hashes, ok := t["hashes"].(map[string]any); ok {
				if h, ok := hashes["sha256"].(string); ok {
					sha256 = h
				}
			}

			apps := make(map[string]string)
			if dockerApps, ok := custom["docker_compose_apps"].(map[string]any); ok {
				for appName, appVal := range dockerApps {
					if appMap, ok := appVal.(map[string]any); ok {
						if uri, ok := appMap["uri"].(string); ok {
							apps[appName] = uri
						}
					}
				}
			}

			latest = &latestTarget{
				Name:    name,
				Version: versionStr,
				Sha256:  sha256,
				Apps:    apps,
			}
		}
	}

	return latest
}

func (h handlers) updatesList(c echo.Context) error {
	var updates map[string][]string
	if err := getJson(c.Request().Context(), "/v1/updates", &updates); err != nil {
		return h.handleUnexpected(c, err)
	}

	ctx := struct {
		baseCtx
		Updates map[string][]string
	}{
		baseCtx: h.baseCtx(c, "Updates", "updates"),
		Updates: updates,
	}
	return h.templates.ExecuteTemplate(c.Response(), "updates.html", ctx)
}

func (h handlers) updatesGet(c echo.Context) error {
	url := fmt.Sprintf("/v1/updates/%s/%s/rollouts", c.Param("tag"), c.Param("name"))

	var rollouts []string
	if err := getJson(c.Request().Context(), url, &rollouts); err != nil {
		return h.handleUnexpected(c, err)
	}

	var groups []string
	if err := getJson(c.Request().Context(), "/v1/known-labels/device-groups", &groups); err != nil {
		return h.handleUnexpected(c, err)
	}

	url = fmt.Sprintf("/v1/updates/%s/%s/tuf", c.Param("tag"), c.Param("name"))
	var tuf api.UpdateTufResp
	tufErr := ""
	if err := getJson(c.Request().Context(), url, &tuf); err != nil {
		tufErr = "Unable to look up the TUF metadata"
	}
	tufJson, err := json.MarshalIndent(tuf, "", "  ")
	if err != nil {
		return h.handleUnexpected(c, err)
	}

	ctx := struct {
		baseCtx
		Tag          string
		Name         string
		Rollouts     []string
		Groups       []string
		Tuf          api.UpdateTufResp
		TufJson      string
		LatestTarget *latestTarget
		TufError     string
	}{
		baseCtx:      h.baseCtx(c, "Update Details", "updates"),
		Tag:          c.Param("tag"),
		Name:         c.Param("name"),
		Rollouts:     rollouts,
		Groups:       groups,
		Tuf:          tuf,
		TufJson:      string(tufJson),
		LatestTarget: findLatestTarget(tuf),
		TufError:     tufErr,
	}
	return h.templates.ExecuteTemplate(c.Response(), "update.html", ctx)
}

func (h handlers) updatesRollout(c echo.Context) error {
	url := fmt.Sprintf("/v1/updates/%s/%s/rollouts/%s", c.Param("tag"), c.Param("name"), c.Param("rollout"))

	var details api.Rollout
	if err := getJson(c.Request().Context(), url, &details); err != nil {
		return EchoError(c, err, 500, err.Error())
	}

	ctx := struct {
		baseCtx
		Tag     string
		Name    string
		Rollout string
		Details api.Rollout
	}{
		baseCtx: h.baseCtx(c, "Rollout Details", "updates"),
		Tag:     c.Param("tag"),
		Name:    c.Param("name"),
		Rollout: c.Param("rollout"),
		Details: details,
	}
	return h.templates.ExecuteTemplate(c.Response(), "update_rollout.html", ctx)
}

func (h handlers) updatesTail(c echo.Context) error {
	ctx := struct {
		baseCtx
		TailUrl string
	}{
		baseCtx: h.baseCtx(c, "Rollout Progress", "updates"),
		TailUrl: fmt.Sprintf("/v1/updates/%s/%s/tail", c.Param("tag"), c.Param("name")),
	}

	return h.templates.ExecuteTemplate(c.Response(), "update_tail.html", ctx)
}

func (h handlers) updatesRolloutTail(c echo.Context) error {
	ctx := struct {
		baseCtx
		TailUrl string
	}{
		baseCtx: h.baseCtx(c, "Rollout Progress", "updates"),
		TailUrl: fmt.Sprintf("/v1/updates/%s/%s/rollouts/%s/tail", c.Param("tag"), c.Param("name"), c.Param("rollout")),
	}

	return h.templates.ExecuteTemplate(c.Response(), "update_tail.html", ctx)
}
