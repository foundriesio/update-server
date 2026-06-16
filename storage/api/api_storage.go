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
	"slices"
	"strings"

	"github.com/foundriesio/update-server/storage"
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
	Update            = storage.Update

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
	OrderBy OrderBy `query:"order-by" default:"last-seen-desc"`
	Limit   int     `query:"limit"    default:"1000"`
	Offset  int     `query:"offset"   default:"0"`
}

type DeviceListItem struct {
	Uuid      string `json:"uuid"`
	CreatedAt int64  `json:"created-at"`
	LastSeen  int64  `json:"last-seen"`
	Target    string `json:"target"`
	Tag       string `json:"tag"`
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

	stmtDeviceCount     stmtDeviceCount
	stmtDeviceDelete    stmtDeviceDelete
	stmtDeviceGet       stmtDeviceGet
	stmtDeviceGetGroups stmtDeviceGetGroups
	stmtDeviceGetLabels stmtDeviceGetLabels
	stmtDeviceList      map[OrderBy]stmtDeviceList
	stmtDeviceSetLabels stmtDeviceSetLabels
	stmtDeviceSetUpdate stmtDeviceSetUpdate
	stmtUpdateInsert    stmtUpdateInsert
	stmtUpdateList      stmtUpdateList
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
		&handle.stmtDeviceCount,
		&handle.stmtDeviceDelete,
		&handle.stmtDeviceGet,
		&handle.stmtDeviceGetGroups,
		&handle.stmtDeviceGetLabels,
		&handle.stmtDeviceSetLabels,
		&handle.stmtDeviceSetUpdate,
		&handle.stmtUpdateInsert,
		&handle.stmtUpdateList,
	); err != nil {
		return nil, err
	}

	handle.stmtDeviceList = make(map[OrderBy]stmtDeviceList, len(orderByDeviceMap))
	for orderBy, orderByStr := range orderByDeviceMap {
		stmt := stmtDeviceList{}
		if err := stmt.Init(*db, orderByStr); err != nil {
			return nil, err
		}
		handle.stmtDeviceList[orderBy] = stmt
	}

	return &handle, nil
}

func (s Storage) DevicesList(opts DeviceListOpts) ([]DeviceListItem, int, error) {
	orderBy := opts.OrderBy
	if orderBy == "" {
		orderBy = OrderByDeviceLastSeenDsc
	}
	stmt, ok := s.stmtDeviceList[orderBy]
	if !ok {
		return nil, 0, fmt.Errorf("invalid order by arg: %s", opts.OrderBy)
	}

	total, err := s.stmtDeviceCount.run()
	if err != nil {
		return nil, 0, err
	}

	devices := make([]DeviceListItem, 0, opts.Limit)
	if err := stmt.run(opts.Limit, opts.Offset, &devices); err != nil {
		return nil, 0, err
	}

	return devices, total, nil
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
		&apps, &labels,
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

func (s Storage) ListUpdates(tag string) (map[string][]Update, error) {
	return s.stmtUpdateList.run(tag)
}

func (s Storage) GetUpdateTufMetadata(tag, updateName string) (map[string]map[string]any, error) {
	handle := s.fs.Updates

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

func (s Storage) ListRollouts(tag, updateName string) ([]string, error) {
	return s.fs.Updates.Rollouts.ListFiles(tag, updateName)
}

func (s Storage) GetRollout(tag, updateName, rolloutName string) (res Rollout, err error) {
	var content string
	content, err = s.fs.Updates.Rollouts.ReadFile(tag, updateName, rolloutName)
	if err == nil {
		err = json.Unmarshal([]byte(content), &res)
	}
	return
}

func (s Storage) SaveRollout(tag, updateName, rolloutName string, rollout Rollout) error {
	if data, err := json.Marshal(rollout); err != nil {
		return err
	} else {
		return s.fs.Updates.Rollouts.WriteFile(tag, updateName, rolloutName, string(data))
	}
}

func (s Storage) CreateRollout(tag, updateName, rolloutName string, rollout Rollout) error {
	h := s.fs.Updates.Rollouts
	log := fmt.Sprintf("%s|%s|%s\n", tag, updateName, rolloutName)
	if data, err := json.Marshal(rollout); err != nil {
		return err
	} else if err := h.AppendJournal(log); err != nil {
		return err
	} else {
		return h.WriteFile(tag, updateName, rolloutName, string(data))
	}
}

func (s Storage) CommitRollout(tag, updateName, rolloutName string, rollout Rollout) (err error) {
	if rollout.Effect, err = s.SetUpdateName(tag, updateName, rollout.Uuids, rollout.Groups); err != nil {
		return err
	} else {
		rollout.Commit = true
		return s.SaveRollout(tag, updateName, rolloutName, rollout)
	}
}

func (s Storage) ReadRolloutJournal() iter.Seq2[*[3]string, error] {
	h := s.fs.Updates.Rollouts
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

func (s Storage) RolloverRolloutJournal() error {
	return s.fs.Updates.Rollouts.RolloverJournal()
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

func (s Storage) SetUpdateName(tag, updateName string, uuids, groups []string) (effectiveUuids []string, err error) {
	err = s.stmtDeviceSetUpdate.run(tag, updateName, uuids, groups, &effectiveUuids)
	return
}

func (s Storage) TailRolloutsLog(tag, updateName string, stop storage.DoneChan) iter.Seq2[string, error] {
	return s.fs.Updates.Logs.TailFileLines(tag, updateName, storage.LogRolloutsFile, stop)
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

func (s Storage) CreateUpdate(tag, updateName string, payload io.Reader) error {
	cleanup := func(cleanupErr error) {
		// This is not critical - log and let the "real" error/success return below.
		slog.Error("Failed to clean upload directory", "error", cleanupErr)
	}
	if err := s.fs.Updates.SaveUpload(tag, updateName, payload, cleanup); err != nil {
		return err
	}
	return s.stmtUpdateInsert.run(tag, updateName)
}

type stmtDeviceGet storage.DbStmt

func (s *stmtDeviceGet) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("apiDeviceGet", `
		SELECT
			created_at, last_seen, pubkey, update_name, tag, target_name, ostree_hash, apps, json(labels)
		FROM devices
		WHERE uuid = ? AND deleted=false`,
	)
	return
}

func (s *stmtDeviceGet) run(
	uuid string,
	createdAt, lastSeen *int64,
	pubkey, updateName, tag, targetName, ostreeHash, apps, labels *string,
) error {
	return s.Stmt.QueryRow(uuid).Scan(
		createdAt, lastSeen, pubkey, updateName, tag, targetName, ostreeHash, apps, labels)
}

type stmtDeviceList storage.DbStmt

func (s *stmtDeviceList) Init(db storage.DbHandle, orderBy string) (err error) {
	s.Stmt, err = db.Prepare("apiDeviceList", fmt.Sprintf(`
		SELECT
			uuid, created_at, last_seen, target_name, tag, json(labels)
		FROM devices
		WHERE deleted=false
		ORDER BY %s LIMIT ? OFFSET ?`, orderBy),
	)
	return
}

func (s *stmtDeviceList) run(limit, offset int, dl *[]DeviceListItem) error {
	if rows, err := s.Stmt.Query(limit, offset); err != nil {
		return err
	} else {
		defer func() {
			if err := rows.Close(); err != nil {
				slog.Error("failed to close rows in device list", "error", err)
			}
		}()
		for rows.Next() {
			var (
				d      DeviceListItem
				labels []byte
			)
			if err = rows.Scan(
				&d.Uuid, &d.CreatedAt, &d.LastSeen, &d.Target, &d.Tag, &labels,
			); err != nil {
				return err
			}
			if err = json.Unmarshal(labels, &d.Labels); err != nil {
				return fmt.Errorf("failed to parse device labels: %w", err)
			}
			*dl = append(*dl, d)
		}
		if err = rows.Err(); err != nil {
			return err
		}
	}
	return nil
}

type stmtDeviceCount storage.DbStmt

func (s *stmtDeviceCount) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("apiDeviceCount", `
		SELECT COUNT(*) FROM devices WHERE deleted=false`,
	)
	return
}

func (s *stmtDeviceCount) run() (count int, err error) {
	err = s.Stmt.QueryRow().Scan(&count)
	return
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
		WHERE tag=? AND (
			uuid IN (SELECT value from json_each(?))
			OR
			group_name IN (SELECT value from json_each(?))
		) RETURNING uuid`,
	)
	return
}

func (s *stmtDeviceSetUpdate) run(tag, updateName string, uuids, groups []string, effectiveUuids *[]string) error {
	uuidsStr, err := json.Marshal(uuids)
	if err != nil {
		return fmt.Errorf("unexpected error marshalling UUIDs to JSON: %w", err)
	}
	groupsStr, err := json.Marshal(groups)
	if err != nil {
		return fmt.Errorf("unexpected error marshalling groups to JSON: %w", err)
	}
	if rows, err := s.Stmt.Query(updateName, tag, uuidsStr, groupsStr); err != nil {
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

type stmtUpdateInsert storage.DbStmt

func (s *stmtUpdateInsert) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("apiUpdateInsert", `
		INSERT INTO updates(tag, name, uploaded_at) VALUES(?, ?, unixepoch('now'))`)
	return
}

func (s *stmtUpdateInsert) run(tag, name string) error {
	_, err := s.Stmt.Exec(tag, name)
	return err
}

// InsertUpdate is intended for unit tests that need to seed the updates table
// without going through the full upload path.
func (s Storage) InsertUpdate(tag, name string) error {
	return s.stmtUpdateInsert.run(tag, name)
}

type stmtUpdateList storage.DbStmt

func (s *stmtUpdateList) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("apiUpdateList", `
		SELECT tag, name, uploaded_at FROM updates
		WHERE (? = '' OR tag = ?)
		ORDER BY tag, uploaded_at, name`)
	return
}

func (s *stmtUpdateList) run(tag string) (map[string][]Update, error) {
	rows, err := s.Stmt.Query(tag, tag)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	res := map[string][]Update{}
	for rows.Next() {
		var u Update
		var t string
		if err = rows.Scan(&t, &u.Name, &u.UploadedAt); err != nil {
			return nil, err
		}
		res[t] = append(res[t], u)
	}
	return res, rows.Err()
}
