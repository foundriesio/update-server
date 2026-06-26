// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/foundriesio/update-server/clock"
	"github.com/foundriesio/update-server/context"
	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/tuf"
)

// GetTufRoot returns the raw JSON bytes of a TUF root metadata file. A version
// of 0 (or less) returns the latest root metadata. The returned error wraps
// os.ErrNotExist when the requested root does not exist.
func (s Storage) GetTufRoot(version int) ([]byte, error) {
	return s.fs.Tuf.ReadRoot(version)
}

type TargetOptions struct {
	AppVersion int               // Becomes custom.version
	HardwareId string            // Becomes custom.hardwareIds[0]
	Name       string            // Becomes custom.name (otherwise the ostree ref name)
	OstreeHash string            // Becomes hashes.sha256 (otherwise the ostree ref content)
	Tag        string            // Becomes custom.tags[0]
	Apps       map[string]string // Becomes docker_compose_apps (app name -> sha256)
	BaseUrl    string            // base URL used to build proxied app/target URIs
}

type composeAppURI struct {
	URI string `json:"uri"`
}

type generatedTargetCustom struct {
	Name              string                   `json:"name"`
	Version           string                   `json:"version"`
	HardwareIds       []string                 `json:"hardwareIds"`
	Tags              []string                 `json:"tags"`
	TargetFormat      string                   `json:"targetFormat"`
	DockerComposeApps map[string]composeAppURI `json:"docker_compose_apps,omitempty"`
	CreatedAt         string                   `json:"createdAt"`
}

func (s Storage) GenerateTufMeta(tufDir string, opts TargetOptions) error {
	// tufVer/tgtVer are the highest existing versions for this tag. The new TUF
	// metadata version is derived below (tufVer + 10); the target version reuses
	// the highest existing value unless AppVersion overrides it.
	tufVer, tgtVer, err := s.getLatestVersions(opts.Tag)
	if err != nil {
		return fmt.Errorf("unable to determine latest target versions: %w", err)
	}
	if opts.AppVersion != 0 {
		tgtVer = opts.AppVersion
	}

	custom := generatedTargetCustom{
		Name:         opts.Name,
		Version:      fmt.Sprintf("%d", tgtVer),
		HardwareIds:  []string{opts.HardwareId},
		Tags:         []string{opts.Tag},
		TargetFormat: "OSTREE",
		CreatedAt:    clock.Now().UTC().Format(time.RFC3339),
	}
	if len(opts.Apps) > 0 {
		custom.DockerComposeApps = make(map[string]composeAppURI)
		for app, hash := range opts.Apps {
			custom.DockerComposeApps[app] = composeAppURI{
				URI: fmt.Sprintf("%s/composeapphack/%s@sha256:%s", opts.BaseUrl, app, hash),
			}
		}
	}

	customJSON, err := json.Marshal(custom)
	if err != nil {
		return fmt.Errorf("unable to marshal generated target custom: %w", err)
	}

	ostreeHash, err := hex.DecodeString(opts.OstreeHash)
	if err != nil {
		return fmt.Errorf("invalid ostree hash %q: %w", opts.OstreeHash, err)
	}

	// We use tufVer + 10 here to allow flexibility for future code to refresh the targets metadata
	// without having to increment the version of the targets metadata for every new target. Adding 10
	// allows us to refresh 10 times with a 90 day expiration allowing us to use a target for almost
	// 900 days if we wanted to.
	targets := tuf.AtsTufTargets{
		Signed: tuf.TargetsMeta{
			SignedCommon: tuf.SignedCommon{
				Type:    tuf.RoleTargets.TufType(),
				Version: tufVer + 10,
				Expires: clock.Now().UTC().Add(s.fs.Tuf.TargetsExpiration).Truncate(time.Second),
			},
			Targets: tuf.TargetFiles{
				fmt.Sprintf("%s-%d", opts.Name, tgtVer): tuf.TargetFileMeta{
					Length: 0,
					Hashes: tuf.Hashes{"sha256": tuf.HexBytes(ostreeHash)},
					Custom: customJSON,
				},
			},
		},
	}

	sig, err := s.fs.Tuf.Sign(tuf.RoleTargets, targets.Signed)
	if err != nil {
		return fmt.Errorf("unable to sign targets metadata: %w", err)
	}
	targets.Signatures = []tuf.Signature{sig}

	ss := tuf.AtsTufSnapshot{
		Signed: tuf.SnapshotMeta{
			SignedCommon: tuf.SignedCommon{
				Type:    tuf.RoleSnapshot.TufType(),
				Version: targets.Signed.Version,
				Expires: targets.Signed.Expires,
			},
			Meta: map[string]tuf.MetaItem{
				storage.TufTargetsFile: {
					Version: targets.Signed.Version,
				},
			},
		},
	}

	sig, err = s.fs.Tuf.Sign(tuf.RoleSnapshot, ss.Signed)
	if err != nil {
		return fmt.Errorf("unable to sign snapshot metadata: %w", err)
	}
	ss.Signatures = []tuf.Signature{sig}

	// When we generate a new timestamp role - we need to increase its version
	// as well in order to let a client (aktualizr) side handle it properly
	// (e.g. store to a local storage). But also, we need to support a tag
	// switch from a lower targets version to a higher one. For a timestamp
	// role to support that we need to provide some "reserve" of timestamp role
	// versions per each snapshot/targets version.  This is why we multiply by
	// 1000 below - that allows to rotate a timestamp role for 8 years every 3
	// days per each targets version.
	tsVersion := targets.Signed.Version * 1000
	ts := tuf.AtsTufTimestamp{
		Signed: tuf.TimestampMeta{
			SignedCommon: tuf.SignedCommon{
				Type:    tuf.RoleTimestamp.TufType(),
				Version: tsVersion,
				Expires: clock.Now().UTC().Add(s.fs.Tuf.TimestampExpiration).Truncate(time.Second),
			},
			Meta: map[string]tuf.MetaItem{
				storage.TufSnapshotFile: {
					Version: ss.Signed.Version,
				},
			},
		},
	}

	sig, err = s.fs.Tuf.Sign(tuf.RoleTimestamp, ts.Signed)
	if err != nil {
		return fmt.Errorf("unable to sign timestamp metadata: %w", err)
	}
	ts.Signatures = []tuf.Signature{sig}

	targetsJson, err := json.Marshal(targets)
	if err != nil {
		return fmt.Errorf("unable to marshal targets metadata: %w", err)
	}
	snapshotJson, err := json.Marshal(ss)
	if err != nil {
		return fmt.Errorf("unable to marshal snapshot metadata: %w", err)
	}
	timestampJson, err := json.Marshal(ts)
	if err != nil {
		return fmt.Errorf("unable to marshal timestamp metadata: %w", err)
	}

	if err := s.fs.Tuf.WriteMeta(tufDir, targetsJson, snapshotJson, timestampJson); err != nil {
		return fmt.Errorf("unable to write targets metadata: %w", err)
	}

	return nil
}

// getLatestVersions returns the highest TUF metadata version and the highest
// target/app version currently present across all updates for the given tag.
// Both are zero when the tag has no existing TUF metadata.
func (s Storage) getLatestVersions(tag string) (tufVersion, targetVersion int, err error) {
	updates, err := s.ListUpdates(tag)
	if err != nil {
		return 0, 0, err
	}
	for _, u := range updates[tag] {
		var targets tuf.AtsTufTargets
		if err := s.fs.Tuf.ReadTufMeta(tag, u.Name, storage.TufTargetsFile, &targets); err != nil {
			// Skip updates that pre-date TUF or whose metadata is missing/unreadable.
			continue
		}
		if tufVersion < targets.Signed.Version {
			tufVersion = targets.Signed.Version
		}
		latest := targets.GetLatestTargetVersion()
		if targetVersion < latest {
			targetVersion = latest
		}
	}
	return tufVersion, targetVersion, nil
}

func (s Storage) RefreshTufTimestamps(c context.Context) error {
	updates, err := s.ListUpdates("")
	if err != nil {
		return err
	}

	log := context.CtxGetLog(c)

	for tag, updates := range updates {
		log.Info("Checking TUF timestamp expiry for tag", "tag", tag, "updates", len(updates))
		for _, u := range updates {
			log.Debug("Checking timestamp for", "tag", tag, "update", u.Name)
			if err := s.refreshTufTimestamp(c, tag, u); err != nil {
				log.Error("Failed to refresh TUF timestamps", "tag", tag, "update", u.Name, "error", err)
			}
		}
	}
	return nil
}

func (s Storage) refreshTufTimestamp(c context.Context, tag string, update Update) error {
	log := context.CtxGetLog(c)

	var ts tuf.AtsTufTimestamp
	if err := s.fs.Tuf.ReadTufMeta(tag, update.Name, storage.TufTimestampFile, &ts); err != nil {
		return fmt.Errorf("unable to read timestamp metadata: %w", err)
	}

	cutoff := clock.Now().UTC().Add(time.Hour * 24) // 1 day from now
	if ts.Signed.Expires.After(cutoff) {
		log.Debug("Timestamp okay", "tag", tag, "update", update.Name, "expiry", ts.Signed.Expires, "cutoff", cutoff)
		return nil // timestamp is still valid, no need to refresh
	}

	ts.Signed.Expires = clock.Now().UTC().Add(s.fs.Tuf.TimestampExpiration).Truncate(time.Second)
	sig, err := s.fs.Tuf.Sign(tuf.RoleTimestamp, ts.Signed)
	if err != nil {
		return fmt.Errorf("unable to sign timestamp metadata: %w", err)
	}
	ts.Signatures = []tuf.Signature{sig}

	tsJson, err := json.Marshal(ts)
	if err != nil {
		return fmt.Errorf("unable to marshal timestamp metadata: %w", err)
	}

	if err := s.fs.Tuf.WriteTimestamp(tag, update.Name, tsJson); err != nil {
		return err
	}

	log.Info("Refreshed TUF timestamp", "tag", tag, "update", update.Name, "new_expiry", ts.Signed.Expires)
	return nil
}
