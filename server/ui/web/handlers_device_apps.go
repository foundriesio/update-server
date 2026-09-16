// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	toml "github.com/pelletier/go-toml"

	"github.com/foundriesio/update-server/server/ui/api"
	"github.com/foundriesio/update-server/storage/tuf"
)

func (h handlers) configsDeviceItemApps(c echo.Context) error {
	uuid := c.Param("uuid")
	var device api.Device
	if err := getJson(c.Request().Context(), "/v1/devices/"+uuid, &device); err != nil {
		return h.handleUnexpected(c, err)
	}
	var configs api.ConfigFileSet
	if err := getJson(c.Request().Context(), "/v1/configs/device/"+uuid, &configs); err != nil {
		return h.handleUnexpected(c, err)
	}
	sotaTag, sotaApps, err := getTagAndAppsFromConfigs(configs)
	if err != nil {
		return h.handleUnexpected(c, err)
	}

	var tagsToAppsMap map[string][]string
	if device.Tag != "" && device.UpdateName != "" {
		updateUrl := fmt.Sprintf("/v1/updates/%s/%s/tuf/targets.json", device.Tag, device.UpdateName)
		var tufTargets tuf.AtsTufTargets
		if err := getJson(c.Request().Context(), updateUrl, &tufTargets); err != nil {
			return h.handleUnexpected(c, err)
		}
		targets := parseTufTargetsForAppsConfig(c, tufTargets.Signed.Targets)
		tagsToAppsMap = getAppsForLatestTagsVersions(targets)
	}

	ctx := struct {
		baseCtx
		UpdateName         string
		OverrideTag        bool
		OverrideApps       bool
		ReportedTag        string
		SelectedTag        string
		SelectedApps       []string
		SupportedTags      []string
		SupportedAppsByTag map[string][]string
		ConfigFileName     string
		CanEdit            bool
	}{
		baseCtx:            h.baseCtx(c, fmt.Sprintf("Device \"%s\" Apps", uuid), "devices"),
		UpdateName:         device.UpdateName,
		OverrideTag:        sotaTag != "",
		OverrideApps:       sotaApps != nil,
		ReportedTag:        device.Tag,
		SelectedTag:        sotaTag,
		SelectedApps:       sotaApps,
		SupportedTags:      slices.Sorted(maps.Keys(tagsToAppsMap)),
		SupportedAppsByTag: tagsToAppsMap,
		ConfigFileName:     api.ConfigSotaOverride,
		CanEdit:            h.configsEditable(c),
	}
	return h.templates.ExecuteTemplate(c.Response(), "device_apps.html", ctx)
}

type target struct {
	version int
	tags    []string
	apps    []string
}

func parseTufTargetsForAppsConfig(c echo.Context, targets tuf.TargetFiles) map[string]target {
	log := CtxGetLog(c.Request().Context())
	parsed := make(map[string]target, len(targets))
	for name, jsonTgt := range targets {
		var tgt target
		var rawTgt struct {
			Version string         `json:"version"`
			Tags    []string       `json:"tags"`
			Apps    map[string]any `json:"docker_compose_apps"`
		}
		logError := func(err error) {
			// Log and continue; ignoring all exotic targets.
			log.Warn("Ignoring invalid target", "error", err, "target", name)
		}
		if err := json.Unmarshal(jsonTgt.Custom, &rawTgt); err != nil {
			logError(err)
			continue
		} else if tgt.version, err = strconv.Atoi(rawTgt.Version); err != nil {
			logError(err)
			continue
		} else {
			tgt.tags = slices.Sorted(slices.Values(rawTgt.Tags))
			tgt.apps = slices.Sorted(maps.Keys(rawTgt.Apps))
		}
		parsed[name] = tgt
	}
	return parsed
}

func getAppsForLatestTagsVersions(targets map[string]target) map[string][]string {
	res := make(map[string][]string, len(targets))
	versions := make(map[string]int, len(targets))
	for _, tgt := range targets {
		for _, tag := range tgt.tags {
			if v, ok := versions[tag]; !ok || v < tgt.version {
				versions[tag] = tgt.version
				res[tag] = tgt.apps
			}
		}
	}
	return res
}

func getTagAndAppsFromConfigs(configs api.ConfigFileSet) (tag string, apps []string, err error) {
	var sota struct {
		Pacman struct {
			Tag  *string `toml:"tags"`
			Apps *string `toml:"compose_apps"`
		} `toml:"pacman"`
	}
	if sotaFile, ok := configs.Files[api.ConfigSotaOverride]; !ok {
		return
	} else if err = toml.Unmarshal([]byte(sotaFile.Value), &sota); err != nil {
		return
	}
	if sota.Pacman.Tag != nil {
		// For tag a nil vs empty value are the same: both mean to inherit, as we require a tag to be non-empty.
		tag = *sota.Pacman.Tag
	}
	if sota.Pacman.Apps != nil {
		// For apps a nil vs empty value are different: nil means to inherit, empty string means to disable all apps.
		apps = make([]string, 0)
		for _, app := range strings.Split(*sota.Pacman.Apps, ",") {
			if len(app) > 0 {
				apps = append(apps, app)
			}
		}
		slices.Sort(apps)
	}
	return
}
