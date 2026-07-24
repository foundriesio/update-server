// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/ostree"
)

// StatusSummary is a summary of the last known state. It includes the total
// number of devices and a sampling (max 100) of device UUIDs in this state.
type StatusSummary struct {
	TotalDevices int      `json:"total-devices"`
	Sampling     []string `json:"sampling"`
}

// MissingDevices is a list of device UUIDs that were expected to participate
// in the rollout but were not found in the rollout log.
type UpdateReport struct {
	Summaries      map[string]*StatusSummary `json:"summaries"`
	MissingDevices []string                  `json:"missing,omitempty"`
}

func (s Storage) updateReport(tag, name string, uuids map[string]any) (*UpdateReport, error) {
	lastStates := make(map[string]string)

	// Collect the last known state for each device in the rollout.
	for line, err := range s.TailRolloutsLog(tag, name, nil) {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// No rollout log yet, return an empty report.
				return &UpdateReport{
					Summaries: make(map[string]*StatusSummary),
				}, nil
			}
			return nil, err
		}
		var status DeviceStatus
		if err = json.Unmarshal([]byte(line), &status); err == nil {
			if uuids != nil {
				if _, ok := uuids[status.Uuid]; !ok {
					continue
				}
			}
			lastStates[status.Uuid] = status.Status
		}
	}

	report := &UpdateReport{
		Summaries: make(map[string]*StatusSummary),
	}
	for uuid := range lastStates {
		summary, ok := report.Summaries[lastStates[uuid]]
		if !ok {
			summary = &StatusSummary{
				Sampling: make([]string, 0, 100),
			}
			report.Summaries[lastStates[uuid]] = summary
		}
		summary.TotalDevices++
		if len(summary.Sampling) >= 100 {
			continue
		}
		summary.Sampling = append(summary.Sampling, uuid)

		if uuids != nil {
			delete(uuids, uuid)
		}
	}
	report.MissingDevices = make([]string, 0, len(uuids))
	for uuid := range uuids {
		report.MissingDevices = append(report.MissingDevices, uuid)
	}

	return report, nil
}

func (s Storage) UpdateReport(tag, name string) (*UpdateReport, error) {
	return s.updateReport(tag, name, nil)
}

func (s Storage) RolloutReport(tag, updateName, rolloutName string) (*UpdateReport, error) {
	rollout, err := s.GetRollout(tag, updateName, rolloutName)
	if err != nil {
		return nil, err
	}
	filter := make(map[string]any, len(rollout.Effect))
	for _, uuid := range rollout.Effect {
		filter[uuid] = nil
	}
	return s.updateReport(tag, updateName, filter)
}

// generateUpdateTuf probes the uploaded ostree/apps content for an update and
// generates its TUF metadata via AddTarget. Values discovered from the upload
// are overridden by any non-zero fields in overrides.
func (s Storage) generateUpdateTuf(updateDir, tag string, overrides TargetOptions) error {
	opts := TargetOptions{
		BaseUrl: overrides.BaseUrl,
		Tag:     tag,
	}

	ostreeDir := filepath.Join(updateDir, storage.UpdatesOstreeDir)
	if isDir(ostreeDir) {
		if err := probeOstree(ostreeDir, &opts); err != nil {
			return fmt.Errorf("unable to probe ostree repo: %w", err)
		}
	}

	appsDir := filepath.Join(updateDir, "apps/apps")
	if isDir(appsDir) {
		if err := probeApps(appsDir, &opts); err != nil {
			return fmt.Errorf("unable to probe apps: %w", err)
		}
	}

	// Caller-provided values override anything probed from the upload.
	if overrides.Name != "" {
		opts.Name = overrides.Name
	}
	if overrides.AppVersion != 0 {
		opts.AppVersion = overrides.AppVersion
	}
	if overrides.HardwareId != "" {
		opts.HardwareId = overrides.HardwareId
	}
	if overrides.OstreeHash != "" {
		opts.OstreeHash = overrides.OstreeHash
	}
	if len(overrides.Apps) > 0 {
		opts.Apps = overrides.Apps
	}
	if opts.OstreeHash == "" {
		// Default to the sha256 of empty content when no ostree image is present.
		opts.OstreeHash = fmt.Sprintf("%x", sha256.Sum256(nil))
	}
	if len(opts.Name) == 0 {
		opts.Name = "default"
	}

	if len(opts.HardwareId) == 0 {
		return fmt.Errorf("unable to determine hardware id from upload")
	}

	slog.Info("Adding TUF target", "tag", tag, "update", updateDir, "opts", opts)
	return s.GenerateTufMeta(filepath.Join(updateDir, "tuf"), opts)
}

// probeOstree inspects an ostree repository to derive target options.
func probeOstree(repoPath string, opts *TargetOptions) error {
	repo := ostree.NewRepo(repoPath)

	heads, err := repo.ListHeads()
	if err != nil {
		return err
	}
	if len(heads) == 0 {
		return fmt.Errorf("no refs found under refs/heads")
	}
	ref := heads[0]
	opts.Name = ref

	if opts.OstreeHash, err = repo.ReadRef(ref); err != nil {
		return err
	}

	if data, err := repo.ReadFile(ref, "/usr/lib/os-release"); err == nil {
		if v := parseKeyValue(string(data), "IMAGE_VERSION"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				opts.AppVersion = n
			}
		}
	}

	if data, err := repo.ReadFile(ref, "/usr/lib/sota/conf.d/40-hardware-id.toml"); err == nil {
		opts.HardwareId = parseKeyValue(string(data), "primary_ecu_hardware_id")
	}
	if opts.HardwareId == "" {
		opts.HardwareId = detectArch(repo, ref)
	}

	return nil
}

// detectArch guesses the hardware id from the image architecture when no
// hardware-id configuration file is present.
func detectArch(repo *ostree.Repo, ref string) string {
	if _, err := repo.ReadFile(ref, "/lib/ld-linux-aarch64.so.1"); err == nil {
		return "arm64-linux"
	}
	return "amd64-linux"
}

// probeApps maps each app sub-directory name to the sha256 of its single entry.
// The upload layout is apps/<app-name>/<sha256>.
func probeApps(appsDir string, opts *TargetOptions) error {
	entries, err := os.ReadDir(appsDir)
	if err != nil {
		return err
	}

	opts.Apps = make(map[string]string)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub, err := os.ReadDir(filepath.Join(appsDir, e.Name()))
		if err != nil {
			return err
		}
		for _, item := range sub {
			opts.Apps[e.Name()] = item.Name() // the entry under the app is the sha256
			break
		}
	}
	return nil
}

// parseKeyValue returns the value for key from KEY=VALUE style content (os-release
// or simple TOML), stripping surrounding quotes and whitespace.
func parseKeyValue(content, key string) string {
	for line := range strings.SplitSeq(content, "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(k) != key {
			continue
		}
		return strings.Trim(strings.TrimSpace(v), `"'`)
	}
	return ""
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
