// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"regexp"
	"slices"
	"strings"

	"github.com/foundriesio/dg-satellite/storage"
)

type (
	Labels  map[string]string
	OrderBy string

	FsHandle = storage.FsHandle

	AppsStates        = storage.AppsStates
	ConfigFile        = storage.ConfigFile
	ConfigFileSet     = storage.ConfigFileSet
	AppliedConfigs    = storage.AppliedConfigs
	DeviceStatus      = storage.DeviceStatus
	DeviceUpdateEvent = storage.DeviceUpdateEvent

	ErrConfigUploadBroken = storage.ErrConfigUploadBroken
)

const (
	OrderByDeviceLastSeenDsc OrderBy = "last-seen-desc"
	OrderByDeviceLastSeenAsc OrderBy = "last-seen-asc"
	OrderByDeviceCreatedDsc  OrderBy = "created-at-desc"
	OrderByDeviceCreatedAsc  OrderBy = "created-at-asc"
	OrderByDeviceNameAsc     OrderBy = "name-asc"
	OrderByDeviceNameDesc    OrderBy = "name-desc"
	OrderByDeviceUuidAsc     OrderBy = "uuid-asc"
	OrderByDeviceUuidDesc    OrderBy = "uuid-desc"

	ConfigHistoryLimit int = 10
	ConfigSotaOverride     = storage.ConfigSotaOverride
)

// LabelComparison defines the comparison operator for label filters.
type LabelComparison string

const (
	LabelCmpEqual       LabelComparison = "eq"
	LabelCmpNotEqual    LabelComparison = "ne"
	LabelCmpContains    LabelComparison = "contains"
	LabelCmpNotContains LabelComparison = "ncontains"
)

// LabelFilter defines a filter on a device's JSONB labels column.
type LabelFilter struct {
	Label      string
	Value      string
	Comparison LabelComparison
}

// validLabelKey ensures label keys only contain safe characters for JSON path embedding.
var validLabelKey = regexp.MustCompile(`^[a-zA-Z0-9_\-]+$`)

func (f LabelFilter) Validate() error {
	switch f.Comparison {
	case LabelCmpEqual, LabelCmpNotEqual, LabelCmpContains, LabelCmpNotContains:
	default:
		return fmt.Errorf("invalid label comparison: %q", f.Comparison)
	}
	if f.Label == "" {
		return fmt.Errorf("label filter key must not be empty")
	}
	if !validLabelKey.MatchString(f.Label) {
		return fmt.Errorf("invalid label filter key: %q", f.Label)
	}
	return nil
}

var orderByDeviceMap = map[OrderBy]string{
	OrderByDeviceCreatedAsc:  "created_at ASC",
	OrderByDeviceCreatedDsc:  "created_at DESC",
	OrderByDeviceLastSeenAsc: "last_seen ASC",
	OrderByDeviceLastSeenDsc: "last_seen DESC",
	// Devices with name always come before devices without name
	OrderByDeviceNameAsc:  "name = '', name ASC NULLS LAST, uuid ASC",
	OrderByDeviceNameDesc: "name = '', name DESC NULLS LAST, uuid DESC",
	OrderByDeviceUuidAsc:  "uuid ASC",
	OrderByDeviceUuidDesc: "uuid DESC",
}

var (
	NewDb = storage.NewDb
	NewFs = storage.NewFs

	DbFile = storage.DbFile

	ValidCorrelationId = storage.ValidCorrelationId
	TestIdRegex        = storage.TestIdRegex

	IsDbError             = storage.IsDbError
	ErrDbConstraintUnique = storage.ErrDbConstraintUnique
	ErrInvalidUpdate      = storage.ErrInvalidUpdate
)

// DeviceListOpts lets you set the order devices will be returned
// by the `List` api
type DeviceListOpts struct {
	OrderBy      OrderBy       `query:"order-by" default:"last-seen-desc"`
	Limit        int           `query:"limit"    default:"1000"`
	Offset       int           `query:"offset"   default:"0"`
	LabelFilters []LabelFilter `json:"label-filters,omitempty"`
}

type DeviceListItem struct {
	Uuid      string `json:"uuid"`
	CreatedAt int64  `json:"created-at"`
	LastSeen  int64  `json:"last-seen"`
	Target    string `json:"target"`
	Tag       string `json:"tag"`
	IsProd    bool   `json:"is-prod"`
	Labels    Labels `json:"labels"`
}

type Device struct {
	DeviceListItem

	Apps       []string `json:"apps"`
	OstreeHash string   `json:"ostree-hash"`
	PubKey     string   `json:"pubkey"`
	UpdateName string   `json:"update-name"`

	Aktoml  string `json:"aktualizr-toml"`
	HwInfo  string `json:"hardware-info"`
	NetInfo string `json:"network-info"`

	Status *DeviceStatus `json:"status,omitempty"`

	storage Storage
}

type Rollout struct {
	Uuids  []string `json:"uuids,omitempty"`
	Groups []string `json:"groups,omitempty"`
	Effect []string `json:"effective-uuids,omitempty"`
	Commit bool     `json:"committed"`
}

type Storage struct {
	db *storage.DbHandle
	fs *storage.FsHandle

	stmtDeviceDelete    stmtDeviceDelete
	stmtDeviceGet       stmtDeviceGet
	stmtDeviceGetGroups stmtDeviceGetGroups
	stmtDeviceGetLabels stmtDeviceGetLabels
	stmtDeviceSetLabels stmtDeviceSetLabels
	stmtDeviceSetUpdate stmtDeviceSetUpdate
}

func (d Device) Delete() error {
	err1 := d.storage.stmtDeviceDelete.run(d.Uuid)
	err2 := d.storage.fs.Devices.Delete(d.Uuid)
	return errors.Join(err1, err2)
}

func (d Device) Updates() ([]string, error) {
	names, err := d.storage.fs.Devices.ListFiles(d.Uuid, storage.EventsPrefix, true)
	if err != nil {
		return nil, err
	}
	for i, name := range names {
		names[i] = name[len(storage.EventsPrefix)+1:]
	}
	slices.Reverse(names)
	return names, nil
}

func (d Device) Events(updateId string) ([]DeviceUpdateEvent, error) {
	name := fmt.Sprintf("%s-%s", storage.EventsPrefix, updateId)
	content, err := d.storage.fs.Devices.ReadFile(d.Uuid, name)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(content, "\n")
	events := make([]DeviceUpdateEvent, 0, len(lines))
	for _, line := range lines {
		if len(line) > 0 {
			var evt DeviceUpdateEvent
			if err := json.Unmarshal([]byte(line), &evt); err != nil {
				return nil, fmt.Errorf("unexpected error unmarshalling event json: %w", err)
			}
			events = append(events, evt)
		}
	}
	return events, nil
}

func (d Device) AppsStates() ([]AppsStates, error) {
	names, err := d.storage.fs.Devices.ListFiles(d.Uuid, storage.StatesPrefix, true)
	if err != nil {
		return nil, err
	}

	states := make([]AppsStates, len(names))
	for i, name := range names {
		content, err := d.storage.fs.Devices.ReadFile(d.Uuid, name)
		if err != nil {
			return nil, err
		}
		var s AppsStates
		if err := json.Unmarshal([]byte(content), &s); err != nil {
			return nil, fmt.Errorf("unexpected error unmarshalling apps states json: %w", err)
		}
		states[len(names)-1-i] = s //store in reverse order
	}
	return states, nil
}

func NewStorage(db *storage.DbHandle, fs *storage.FsHandle) (*Storage, error) {
	handle := Storage{db: db, fs: fs}

	if err := db.InitStmt(
		&handle.stmtDeviceDelete,
		&handle.stmtDeviceGet,
		&handle.stmtDeviceGetGroups,
		&handle.stmtDeviceGetLabels,
		&handle.stmtDeviceSetLabels,
		&handle.stmtDeviceSetUpdate,
	); err != nil {
		return nil, err
	}

	return &handle, nil
}

func (s Storage) DevicesList(opts DeviceListOpts) ([]DeviceListItem, int, error) {
	orderBy := opts.OrderBy
	if orderBy == "" {
		orderBy = OrderByDeviceLastSeenDsc
	}
	orderByStr, ok := orderByDeviceMap[orderBy]
	if !ok {
		return nil, 0, fmt.Errorf("invalid order by arg: %s", opts.OrderBy)
	}

	filterSQL, filterArgs, err := buildLabelFilterSQL(opts.LabelFilters)
	if err != nil {
		return nil, 0, err
	}

	// Count query
	countQuery := "SELECT COUNT(*) FROM devices WHERE deleted=false" + filterSQL
	var total int
	if err := s.db.QueryRow(countQuery, filterArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count devices: %w", err)
	}

	// List query
	listQuery := fmt.Sprintf(
		"SELECT uuid, created_at, last_seen, target_name, tag, is_prod, json(labels) FROM devices WHERE deleted=false%s ORDER BY %s LIMIT ? OFFSET ?",
		filterSQL, orderByStr,
	)
	listArgs := append(filterArgs, opts.Limit, opts.Offset)
	rows, err := s.db.Query(listQuery, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list devices: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error("failed to close rows in device list", "error", err)
		}
	}()

	devices := make([]DeviceListItem, 0, opts.Limit)
	for rows.Next() {
		var (
			d      DeviceListItem
			labels []byte
		)
		if err = rows.Scan(
			&d.Uuid, &d.CreatedAt, &d.LastSeen, &d.Target, &d.Tag, &d.IsProd, &labels,
		); err != nil {
			return nil, 0, err
		}
		if err = json.Unmarshal(labels, &d.Labels); err != nil {
			return nil, 0, fmt.Errorf("failed to parse device labels: %w", err)
		}
		devices = append(devices, d)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, err
	}

	return devices, total, nil
}

// buildLabelFilterSQL produces SQL WHERE clause fragments and args for label filters.
// Each filter uses the JSONB extract operator (labels ->> '$.<key>') with parameterized values.
// For "name" and "group" labels, it uses the indexed virtual columns directly.
func buildLabelFilterSQL(filters []LabelFilter) (string, []any, error) {
	if len(filters) == 0 {
		return "", nil, nil
	}

	var sb strings.Builder
	args := make([]any, 0, len(filters))
	for _, f := range filters {
		if err := f.Validate(); err != nil {
			return "", nil, err
		}

		// Use indexed virtual columns for "name" and "group" labels;
		// fall back to JSON extraction for all other labels.
		var col string
		switch f.Label {
		case "name":
			col = "name"
		case "group":
			col = "group_name"
		default:
			col = fmt.Sprintf("labels ->> '$.%s'", f.Label)
		}

		switch f.Comparison {
		case LabelCmpEqual:
			fmt.Fprintf(&sb, " AND %s = ?", col)
			args = append(args, f.Value)
		case LabelCmpNotEqual:
			fmt.Fprintf(&sb, " AND (%s IS NULL OR %s != ?)", col, col)
			args = append(args, f.Value)
		case LabelCmpContains:
			fmt.Fprintf(&sb, " AND %s LIKE ?", col)
			args = append(args, "%"+f.Value+"%")
		case LabelCmpNotContains:
			fmt.Fprintf(&sb, " AND (%s IS NULL OR %s NOT LIKE ?)", col, col)
			args = append(args, "%"+f.Value+"%")
		}
	}
	return sb.String(), args, nil
}

func (s Storage) DeviceGet(uuid string) (*Device, error) {
	d := Device{storage: s, DeviceListItem: DeviceListItem{Uuid: uuid}}
	var (
		err    error
		apps   string
		labels string
	)
	if err := s.stmtDeviceGet.run(
		uuid,
		&d.CreatedAt, &d.LastSeen,
		&d.PubKey, &d.UpdateName, &d.Tag, &d.Target, &d.OstreeHash,
		&apps, &labels, &d.IsProd,
	); err != nil {
		if err == sql.ErrNoRows {
			err = nil
		}
		return nil, err
	}
	for _, v := range strings.Split(apps, ",") {
		if v = strings.TrimSpace(v); len(v) > 0 {
			d.Apps = append(d.Apps, v)
		}
	}
	if err = json.Unmarshal([]byte(labels), &d.Labels); err != nil {
		return nil, fmt.Errorf("failed to parse device labels: %w", err)
	}

	if d.Aktoml, err = s.fs.Devices.ReadFile(d.Uuid, storage.AktomlFile); err != nil {
		return nil, err
	}
	if d.HwInfo, err = s.fs.Devices.ReadFile(d.Uuid, storage.HwInfoFile); err != nil {
		return nil, err
	}
	if d.NetInfo, err = s.fs.Devices.ReadFile(d.Uuid, storage.NetInfoFile); err != nil {
		return nil, err
	}

	// Find the most recent update event and derive device status
	eventFiles, err := s.fs.Devices.ListFiles(d.Uuid, storage.EventsPrefix, true)
	if err != nil {
		return nil, err
	}
	if len(eventFiles) > 0 {
		content, err := s.fs.Devices.ReadFile(d.Uuid, eventFiles[len(eventFiles)-1])
		if err != nil {
			return nil, err
		}
		content = strings.TrimSpace(content)
		lastLine := content[strings.LastIndexByte(content, '\n')+1:]
		var evt DeviceUpdateEvent
		if err := json.Unmarshal([]byte(lastLine), &evt); err != nil {
			return nil, fmt.Errorf("unexpected error unmarshalling event json: %w", err)
		}

		if !slices.Contains(clearingEventTypes, evt.EventType.Id) || evt.Event.Success == nil || !*evt.Event.Success {
			// only share status if its interesting (ie not a successful update)
			status := evt.ParseStatus()
			d.Status = &status
		}
	}

	return &d, nil
}

func (s Storage) ReadAppliedConfigs(uuid string) (*storage.AppliedConfigs, error) {
	raw, err := s.fs.Devices.ReadFile(uuid, storage.ConfigAppliedFile)
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, nil
	}
	var applied storage.AppliedConfigs
	if err := json.Unmarshal([]byte(raw), &applied); err != nil {
		return nil, fmt.Errorf("failed to parse applied config: %w", err)
	}
	return &applied, nil
}

var clearingEventTypes = []string{"EcuInstallationCompleted", "CertRotationCompleted", "MetadataUpdateCompleted"}

func (s Storage) ListUpdates(tag string, isProd bool) (map[string][]string, error) {
	return s.getRolloutsFsHandle(isProd).ListUpdates(tag)
}

func (s Storage) GetUpdateTufMetadata(tag, updateName string, isProd bool) (map[string]map[string]any, error) {
	handle := s.fs.Updates.Ci
	if isProd {
		handle = s.fs.Updates.Prod
	}

	latestRoot, err := handle.Tuf.LatestRootMetaName(tag, updateName)
	if err != nil {
		return nil, err
	}

	meta := make(map[string]map[string]any)
	for _, x := range []string{storage.TufTargetsFile, storage.TufSnapshotFile, storage.TufTimestampFile, latestRoot} {
		metaStr, err := handle.Tuf.ReadFile(tag, updateName, x)
		if err != nil {
			return nil, err
		}
		var metaDict map[string]any
		if err := json.Unmarshal([]byte(metaStr), &metaDict); err != nil {
			return nil, fmt.Errorf("failed to unmarshal %s: %w", x, err)
		}
		if x == latestRoot {
			x = storage.TufRootFile
		}
		meta[x] = metaDict
	}

	return meta, nil
}

func (s Storage) ListRollouts(tag, updateName string, isProd bool) ([]string, error) {
	return s.getRolloutsFsHandle(isProd).ListFiles(tag, updateName)
}

func (s Storage) GetRollout(tag, updateName, rolloutName string, isProd bool) (res Rollout, err error) {
	var content string
	content, err = s.getRolloutsFsHandle(isProd).ReadFile(tag, updateName, rolloutName)
	if err == nil {
		err = json.Unmarshal([]byte(content), &res)
	}
	return
}

func (s Storage) SaveRollout(tag, updateName, rolloutName string, isProd bool, rollout Rollout) error {
	if data, err := json.Marshal(rollout); err != nil {
		return err
	} else {
		return s.getRolloutsFsHandle(isProd).WriteFile(tag, updateName, rolloutName, string(data))
	}
}

func (s Storage) CreateRollout(tag, updateName, rolloutName string, isProd bool, rollout Rollout) error {
	h := s.getRolloutsFsHandle(isProd)
	log := fmt.Sprintf("%s|%s|%s\n", tag, updateName, rolloutName)
	if data, err := json.Marshal(rollout); err != nil {
		return err
	} else if err := h.AppendJournal(log); err != nil {
		return err
	} else {
		return h.WriteFile(tag, updateName, rolloutName, string(data))
	}
}

func (s Storage) CommitRollout(tag, updateName, rolloutName string, isProd bool, rollout Rollout) (err error) {
	if rollout.Effect, err = s.SetUpdateName(tag, updateName, isProd, rollout.Uuids, rollout.Groups); err != nil {
		return err
	} else {
		rollout.Commit = true
		return s.SaveRollout(tag, updateName, rolloutName, isProd, rollout)
	}
}

func (s Storage) ReadRolloutJournal(isProd bool) iter.Seq2[*[3]string, error] {
	h := s.getRolloutsFsHandle(isProd)
	return func(yield func(*[3]string, error) bool) {
		for log, err := range h.ReadJournal() {
			if err != nil {
				yield(nil, err)
				break
			}
			parts := strings.Split(log, "|")
			if len(parts) != 3 {
				// This is impossible; just a sanity check.
				yield(nil, fmt.Errorf("corrupted journal file line: %s", log))
				break
			}
			// parts are tag, updateName, rolloutName
			if !yield(&[3]string{parts[0], parts[1], parts[2]}, nil) {
				break
			}
		}
	}
}

func (s Storage) RolloverRolloutJournal(isProd bool) error {
	return s.getRolloutsFsHandle(isProd).RolloverJournal()
}

func (s Storage) GetKnownDeviceGroupNames() ([]string, error) {
	if dbNames, err := s.stmtDeviceGetGroups.run(); err != nil {
		return nil, err
	} else if fsNames, err := s.fs.Configs.ReadGroupNames(); err != nil {
		return nil, err
	} else {
		// Reuse fsNames for the final result, using search-and-sort-in-place technique.
		// Golang warrants that dir entry names are sorted alphabetically.
		for _, name := range dbNames {
			if idx, has := slices.BinarySearch(fsNames, name); !has {
				fsNames = slices.Insert(fsNames, idx, name)
			}
		}
		return fsNames, nil
	}
}

func (s Storage) GetKnownDeviceLabelNames() ([]string, error) {
	return s.stmtDeviceGetLabels.run()
}

func (s Storage) PatchDeviceLabels(labels map[string]*string, uuids []string) error {
	// This function applies a merge-patch on top of existing labels:
	// new labels are added, updated labels are replaced, null labels are removed, missing labels are left intact.
	return s.stmtDeviceSetLabels.run(labels, uuids)
}

func (s Storage) SetUpdateName(tag, updateName string, isProd bool, uuids, groups []string) (effectiveUuids []string, err error) {
	err = s.stmtDeviceSetUpdate.run(tag, updateName, isProd, uuids, groups, &effectiveUuids)
	return
}

func (s Storage) TailRolloutsLog(tag, updateName string, isProd bool, stop storage.DoneChan) iter.Seq2[string, error] {
	fs := s.fs.Updates.Ci.Logs
	if isProd {
		fs = s.fs.Updates.Prod.Logs
	}
	return fs.TailFileLines(tag, updateName, storage.LogRolloutsFile, stop)
}

func (s Storage) ReadFactoryConfigHistory(latest int, withFiles bool) ([]*ConfigFileSet, error) {
	return s.fs.Configs.ReadFactoryConfigHistory(latest, withFiles)
}

func (s Storage) SaveFactoryConfig(content, username, reason string) error {
	if err := s.fs.Configs.WriteFactoryConfig(content, username, reason); err != nil {
		return err
	} else if err = s.fs.Configs.PurgeFactoryConfigHistory(ConfigHistoryLimit); err != nil {
		slog.Error("Failed to clean factory config history", "error", err)
	}
	return nil
}

func (s Storage) ReadGroupConfigHistory(name string, latest int, withFiles bool) ([]*ConfigFileSet, error) {
	return s.fs.Configs.ReadGroupConfigHistory(name, latest, withFiles)
}

func (s Storage) SaveGroupConfig(name, content, username, reason string) error {
	if err := s.fs.Configs.WriteGroupConfig(name, content, username, reason); err != nil {
		return err
	} else if err = s.fs.Configs.PurgeGroupConfigHistory(name, ConfigHistoryLimit); err != nil {
		slog.Error("Failed to clean group config history", "group", name, "error", err)
	}
	return nil
}

func (s Storage) ReadDeviceConfigHistory(uuid string, latest int, withFiles bool) ([]*ConfigFileSet, error) {
	return s.fs.Configs.ReadDeviceConfigHistory(uuid, latest, withFiles)
}

func (s Storage) SaveDeviceConfig(uuid, content, username, reason string) error {
	if err := s.fs.Configs.WriteDeviceConfig(uuid, content, username, reason); err != nil {
		return err
	} else if err = s.fs.Configs.PurgeDeviceConfigHistory(uuid, ConfigHistoryLimit); err != nil {
		slog.Error("Failed to clean device config history", "uuid", uuid, "error", err)
	}
	return nil
}

func (s Storage) UploadConfigs(payload io.Reader) (err error) {
	return s.fs.Configs.SaveUpload(payload, func(cleanupErr error) {
		// This is not critical - log and let the "real" error/success return below.
		slog.Error("Failed to clean upload directory", "error", cleanupErr)
	})
}

func (s Storage) CreateUpdate(tag, updateName string, isProd bool, payload io.Reader) error {
	cleanup := func(cleanupErr error) {
		// This is not critical - log and let the "real" error/success return below.
		slog.Error("Failed to clean upload directory", "error", cleanupErr)
	}
	if isProd {
		return s.fs.Updates.Prod.SaveUpload(tag, updateName, payload, cleanup)
	} else {
		return s.fs.Updates.Ci.SaveUpload(tag, updateName, payload, cleanup)
	}
}

func (s Storage) getRolloutsFsHandle(isProd bool) storage.RolloutsFsHandle {
	if isProd {
		return s.fs.Updates.Prod.Rollouts
	} else {
		return s.fs.Updates.Ci.Rollouts
	}
}

type stmtDeviceGet storage.DbStmt

func (s *stmtDeviceGet) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("apiDeviceGet", `
		SELECT
			created_at, last_seen, pubkey, update_name, tag, target_name, ostree_hash, apps, json(labels), is_prod
		FROM devices
		WHERE uuid = ? AND deleted=false`,
	)
	return
}

func (s *stmtDeviceGet) run(
	uuid string,
	createdAt, lastSeen *int64,
	pubkey, updateName, tag, targetName, ostreeHash, apps, labels *string,
	isProd *bool,
) error {
	return s.Stmt.QueryRow(uuid).Scan(
		createdAt, lastSeen, pubkey, updateName, tag, targetName, ostreeHash, apps, labels, isProd)
}

type stmtDeviceSetLabels storage.DbStmt

func (s *stmtDeviceSetLabels) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("apiDeviceSetName", `
		UPDATE devices
		SET labels=jsonb_patch(labels,?)
		WHERE uuid IN (SELECT value from json_each(?))`,
	)
	return
}

func (s *stmtDeviceSetLabels) run(labels map[string]*string, uuids []string) error {
	labelsStr, err := json.Marshal(labels)
	if err != nil {
		return fmt.Errorf("unexpected error marshalling labels to JSON: %w", err)
	}
	uuidsStr, err := json.Marshal(uuids)
	if err != nil {
		return fmt.Errorf("unexpected error marshalling UUIDs to JSON: %w", err)
	}
	_, err = s.Stmt.Exec(labelsStr, uuidsStr)
	return err
}

type stmtDeviceGetLabels storage.DbStmt

func (s *stmtDeviceGetLabels) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("apiDeviceGetLabelNames", `SELECT json_group_array(label) FROM device_labels`)
	return
}

func (s *stmtDeviceGetLabels) run() (labels []string, err error) {
	var labelsStr []byte
	if err = s.Stmt.QueryRow().Scan(&labelsStr); err == nil {
		err = json.Unmarshal(labelsStr, &labels)
	}
	return
}

type stmtDeviceGetGroups storage.DbStmt

func (s *stmtDeviceGetGroups) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("apiDeviceGetLabelNames", `
		SELECT json_group_array(DISTINCT group_name) FROM devices
		WHERE group_name != ""
	`)
	return
}

func (s *stmtDeviceGetGroups) run() (groups []string, err error) {
	var groupsStr []byte
	if err = s.Stmt.QueryRow().Scan(&groupsStr); err == nil {
		err = json.Unmarshal(groupsStr, &groups)
	}
	return
}

type stmtDeviceSetUpdate storage.DbStmt

func (s *stmtDeviceSetUpdate) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("apiDeviceSetUpdateName", `
		UPDATE devices
		SET update_name=?
		WHERE tag=? AND is_prod=? AND (
			uuid IN (SELECT value from json_each(?))
			OR
			group_name IN (SELECT value from json_each(?))
		) RETURNING uuid`,
	)
	return
}

func (s *stmtDeviceSetUpdate) run(tag, updateName string, isProd bool, uuids, groups []string, effectiveUuids *[]string) error {
	uuidsStr, err := json.Marshal(uuids)
	if err != nil {
		return fmt.Errorf("unexpected error marshalling UUIDs to JSON: %w", err)
	}
	groupsStr, err := json.Marshal(groups)
	if err != nil {
		return fmt.Errorf("unexpected error marshalling groups to JSON: %w", err)
	}
	if rows, err := s.Stmt.Query(updateName, tag, isProd, uuidsStr, groupsStr); err != nil {
		return err
	} else {
		var resUuid string
		for rows.Next() {
			if err = rows.Scan(&resUuid); err != nil {
				return err
			}
			*effectiveUuids = append(*effectiveUuids, resUuid)
		}
		if err = rows.Err(); err != nil {
			return err
		}
	}
	return nil
}

type stmtDeviceDelete storage.DbStmt

func (s *stmtDeviceDelete) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("apiDeviceDelete", `
		UPDATE devices SET deleted=1 WHERE uuid=?`)
	return
}

func (s *stmtDeviceDelete) run(uuid string) error {
	_, err := s.Stmt.Exec(uuid)
	return err
}
