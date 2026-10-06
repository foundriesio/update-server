// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/foundriesio/update-server/server/ui/api"
	"github.com/labstack/echo/v4"
)

// releaseApplication is one docker-compose app bundled in a release, ready to
// render deterministically (Go maps have no stable iteration order).
type releaseApplication struct {
	Name string
	URI  string
}

type latestTarget struct {
	Name         string
	Version      string
	Sha256       string
	Architecture string
	HardwareIDs  []string
	Tags         []string
	Apps         []releaseApplication
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
		if version <= latestVersion {
			continue
		}
		latestVersion = version

		sha256 := ""
		if hashes, ok := t["hashes"].(map[string]any); ok {
			if h, ok := hashes["sha256"].(string); ok {
				sha256 = h
			}
		}

		arch, _ := custom["arch"].(string)

		var apps []releaseApplication
		if dockerApps, ok := custom["docker_compose_apps"].(map[string]any); ok {
			for appName, appVal := range dockerApps {
				if appMap, ok := appVal.(map[string]any); ok {
					if uri, ok := appMap["uri"].(string); ok {
						apps = append(apps, releaseApplication{Name: appName, URI: uri})
					}
				}
			}
		}
		slices.SortFunc(apps, func(a, b releaseApplication) int { return strings.Compare(a.Name, b.Name) })

		latest = &latestTarget{
			Name:         name,
			Version:      versionStr,
			Sha256:       sha256,
			Architecture: arch,
			HardwareIDs:  customStrings(custom, "hardwareIds"),
			Tags:         customStrings(custom, "tags"),
			Apps:         apps,
		}
	}

	return latest
}

// customStrings returns the string values of a target.custom []any field, in
// their original order.
func customStrings(custom map[string]any, field string) []string {
	values, ok := custom[field].([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if s, ok := value.(string); ok && s != "" {
			result = append(result, s)
		}
	}
	return result
}

// tufMetadataItem is one TUF role's expiration, ready to render in a fixed
// order regardless of which roles are present in the raw metadata.
type tufMetadataItem struct {
	Name    string
	Expires string
}

// releaseTufMetadata returns the standard TUF roles in a fixed display order;
// a role missing from tuf renders with an empty expiration rather than being
// omitted or panicking.
func releaseTufMetadata(tuf api.UpdateTufResp) []tufMetadataItem {
	roles := []struct {
		Name string
		File string
	}{
		{"Root", "root.json"},
		{"Timestamp", "timestamp.json"},
		{"Snapshot", "snapshot.json"},
		{"Targets", "targets.json"},
	}

	items := make([]tufMetadataItem, len(roles))
	for i, role := range roles {
		var expires string
		if signed, ok := tuf[role.File]["signed"].(map[string]any); ok {
			expires, _ = signed["expires"].(string)
		}
		items[i] = tufMetadataItem{Name: role.Name, Expires: expires}
	}
	return items
}

// rolloutSummary is a truthful, template-ready view of one rollout: the API
// exposes no completion/success counts, so this only carries what's known.
type rolloutSummary struct {
	Name        string
	State       string
	DeviceCount int
}

// rolloutSummaryFrom maps storage.Rollout's Commit flag to a human state and
// DeviceCount to len(Effect) — the count of devices actually targeted, never
// len(Uuids)/len(Groups), which are just the rollout's selection criteria.
func rolloutSummaryFrom(name string, rollout api.Rollout) rolloutSummary {
	state := "Preparing"
	if rollout.Commit {
		state = "Scheduled"
	}
	return rolloutSummary{Name: name, State: state, DeviceCount: len(rollout.Effect)}
}

func (h handlers) updatesList(c echo.Context) error {
	var updates []api.Update
	if err := getJson(c.Request().Context(), "/v1/updates", &updates); err != nil {
		return h.handleUnexpected(c, err)
	}

	ctx := struct {
		baseCtx
		Updates []api.Update
	}{
		baseCtx: h.baseCtx(c, "Updates", "updates"),
		Updates: updates,
	}
	return h.templates.ExecuteTemplate(c.Response(), "updates.html", ctx)
}

func (h handlers) updatesGet(c echo.Context) error {
	url := fmt.Sprintf("/v1/updates/%s/rollouts", c.Param("name"))

	var rollouts []string
	if err := getJson(c.Request().Context(), url, &rollouts); err != nil {
		return h.handleUnexpected(c, err)
	}

	var summary api.UpdateSummary
	if err := getJson(c.Request().Context(), fmt.Sprintf("/v1/updates/%s/summary", c.Param("name")), &summary); err != nil {
		return h.handleUnexpected(c, err)
	}

	var groups []string
	if err := getJson(c.Request().Context(), "/v1/known-labels/device-groups", &groups); err != nil {
		return h.handleUnexpected(c, err)
	}

	url = fmt.Sprintf("/v1/updates/%s/tuf", c.Param("name"))
	var tuf api.UpdateTufResp
	tufErr := ""
	if err := getJson(c.Request().Context(), url, &tuf); err != nil {
		tufErr = "Unable to look up the TUF metadata"
	}
	tufJson, err := json.MarshalIndent(tuf, "", "  ")
	if err != nil {
		return h.handleUnexpected(c, err)
	}

	// ListRollouts (backing "/v1/updates/{name}/rollouts") sorts by mod time,
	// so the last entry is the most recently created rollout.
	summaries := make([]rolloutSummary, len(rollouts))
	for i, name := range rollouts {
		var rollout api.Rollout
		rolloutUrl := fmt.Sprintf("/v1/updates/%s/rollouts/%s", c.Param("name"), name)
		if err := getJson(c.Request().Context(), rolloutUrl, &rollout); err != nil {
			return h.handleUnexpected(c, err)
		}
		summaries[i] = rolloutSummaryFrom(name, rollout)
	}
	var latestRollout *rolloutSummary
	if len(summaries) > 0 {
		latestRollout = &summaries[len(summaries)-1]
	}

	ctx := struct {
		baseCtx
		Tag           string
		Name          string
		Summary       api.UpdateSummary
		Rollouts      []rolloutSummary
		LatestRollout *rolloutSummary
		Groups        []string
		Tuf           api.UpdateTufResp
		TufJson       string
		LatestTarget  *latestTarget
		TufMetadata   []tufMetadataItem
		TufError      string
	}{
		baseCtx:       h.baseCtx(c, "Update Details", "updates"),
		Tag:           summary.Tag,
		Name:          c.Param("name"),
		Summary:       summary,
		Rollouts:      summaries,
		LatestRollout: latestRollout,
		Groups:        groups,
		Tuf:           tuf,
		TufJson:       string(tufJson),
		LatestTarget:  findLatestTarget(tuf),
		TufMetadata:   releaseTufMetadata(tuf),
		TufError:      tufErr,
	}
	return h.templates.ExecuteTemplate(c.Response(), "update.html", ctx)
}

func (h handlers) updatesRollout(c echo.Context) error {
	url := fmt.Sprintf("/v1/updates/%s/rollouts/%s", c.Param("name"), c.Param("rollout"))

	var details api.Rollout
	if err := getJson(c.Request().Context(), url, &details); err != nil {
		return EchoError(c, err, 500, err.Error())
	}

	var summary api.UpdateSummary
	if err := getJson(c.Request().Context(), url+"/summary", &summary); err != nil {
		return h.handleUnexpected(c, err)
	}

	ctx := struct {
		baseCtx
		Tag     string
		Name    string
		Rollout string
		Details api.Rollout
		Summary api.UpdateSummary
	}{
		baseCtx: h.baseCtx(c, "Rollout Details", "updates"),
		Tag:     summary.Tag,
		Name:    c.Param("name"),
		Rollout: c.Param("rollout"),
		Details: details,
		Summary: summary,
	}
	return h.templates.ExecuteTemplate(c.Response(), "update_rollout.html", ctx)
}

type tailStreamCtx struct {
	baseCtx
	Name    string
	Rollout string
	TailUrl string
}

func (h handlers) updatesTail(c echo.Context) error {
	ctx := tailStreamCtx{
		baseCtx: h.baseCtx(c, "Update Progress", "updates"),
		Name:    c.Param("name"),
		TailUrl: fmt.Sprintf("/v1/updates/%s/tail", c.Param("name")),
	}

	return h.templates.ExecuteTemplate(c.Response(), "update_tail.html", ctx)
}

func (h handlers) updatesRolloutTail(c echo.Context) error {
	ctx := tailStreamCtx{
		baseCtx: h.baseCtx(c, "Rollout Progress", "updates"),
		Name:    c.Param("name"),
		Rollout: c.Param("rollout"),
		TailUrl: fmt.Sprintf("/v1/updates/%s/rollouts/%s/tail", c.Param("name"), c.Param("rollout")),
	}

	return h.templates.ExecuteTemplate(c.Response(), "rollout_tail.html", ctx)
}
