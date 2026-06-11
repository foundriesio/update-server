// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package gateway

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/foundriesio/update-server/storage"
)

type (
	// Convenience aliases for importing modules
	DbHandle = storage.DbHandle
	FsHandle = storage.FsHandle

	AppliedConfigs    = storage.AppliedConfigs
	AppsStates        = storage.AppsStates
	ConfigFile        = storage.ConfigFile
	DeviceUpdateEvent = storage.DeviceUpdateEvent
)

var (
	NewDb = storage.NewDb
	NewFs = storage.NewFs

	TestIdRegex        = storage.TestIdRegex
	ValidCorrelationId = storage.ValidCorrelationId
)

const (
	// TLS certs
	CertsCasPemFile = storage.CertsCasPemFile
	CertsTlsKeyFile = storage.CertsTlsKeyFile
	CertsTlsPemFile = storage.CertsTlsPemFile

	// Per device files/dirs
	ConfigAppliedFile = storage.ConfigAppliedFile
	AktomlFile        = storage.AktomlFile
	HwInfoFile        = storage.HwInfoFile
	NetInfoFile       = storage.NetInfoFile

	EventsPrefix = storage.EventsPrefix
	StatesPrefix = storage.StatesPrefix

	// Per update files/dirs
	TufRootFile      = storage.TufRootFile
	TufTimestampFile = storage.TufTimestampFile
	TufSnapshotFile  = storage.TufSnapshotFile
	TufTargetsFile   = storage.TufTargetsFile

	// Per config class files/dirs
	ConfigSotaOverride = storage.ConfigSotaOverride
)

type Storage struct {
	db *DbHandle
	fs *FsHandle

	stmtDeviceCheckIn stmtDeviceCheckIn
	stmtDeviceCreate  stmtDeviceCreate
	stmtDeviceGet     stmtDeviceGet

	maxEvents int
	maxStates int
}

type Device struct {
	storage Storage

	Uuid       string `json:"uuid"`
	Apps       string `json:"docker_apps"`
	Deleted    bool   `json:"-"`
	GroupName  string `json:"group_name"`
	IsProd     bool   `json:"is_prod"`
	LastSeen   int64  `json:"last_seen"`
	OstreeHash string `json:"ostree_hash"`
	PubKey     string `json:"pubkey"`
	TargetName string `json:"target_name"`
	Tag        string `json:"tag"`
	UpdateName string `json:"update_name"`

	groupNameModifiedAt int64
}

func (d *Device) CheckIn(targetName, tag, ostreeHash string, apps string) error {
	now := time.Now().Unix()
	if apps == d.Apps && ostreeHash == d.OstreeHash && tag == d.Tag && targetName == d.TargetName && now-d.LastSeen < 60 {
		// Skip database updating when all fields are the same and last checkin was less than a minute ago.
		return nil
	}
	// Update in-memory device object fields to new actual values
	d.Apps = apps
	d.LastSeen = now
	d.OstreeHash = ostreeHash
	d.Tag = tag
	d.TargetName = targetName
	return d.storage.stmtDeviceCheckIn.run(d.Uuid, targetName, tag, ostreeHash, apps, now)
}

func (d *Device) PutFile(name string, content string) error {
	return d.storage.fs.Devices.WriteFile(d.Uuid, name, content)
}

func (d Device) ProcessEvents(events []storage.DeviceUpdateEvent) error {
	var corrId string
	for _, evt := range events {
		if corrId != "" && corrId != evt.Event.CorrelationId {
			// Events ordering depends onto ModTime.
			// Make sure that a later events file gets a later ModTime.
			// Tests show that filesystem's time precision is good enough for 4 milliseconds delay.
			time.Sleep(4 * time.Millisecond)
		}
		corrId = evt.Event.CorrelationId
		name := fmt.Sprintf("%s-%s", storage.EventsPrefix, corrId)
		bytes, err := json.Marshal(evt)
		if err != nil {
			return err
		}
		if err := d.storage.fs.Devices.AppendFile(d.Uuid, name, string(bytes)+"\n"); err != nil {
			return err
		}
		if status := evt.ParseStatus(); len(d.UpdateName) > 0 && len(d.Tag) > 0 {
			status.Uuid = d.Uuid
			bytes, err = json.Marshal(status)
			if err != nil {
				return err
			}
			fs := d.storage.fs.Updates.Ci.Logs
			if d.IsProd {
				fs = d.storage.fs.Updates.Prod.Logs
			}
			if err = fs.AppendFile(d.Tag, d.UpdateName, storage.LogRolloutsFile, string(bytes)+"\n"); err != nil {
				return err
			}
		}
	}
	return d.storage.fs.Devices.RolloverFiles(d.Uuid, storage.EventsPrefix, d.storage.maxEvents)
}

func (d Device) SaveAppsStates(content string) error {
	// Apps states ordering depends onto ModTime.
	// Make sure that a later events file gets a later ModTime.
	time.Sleep(4 * time.Millisecond)
	name := fmt.Sprintf("%s-%d", storage.StatesPrefix, time.Now().UnixMilli())
	if err := d.storage.fs.Devices.WriteFile(d.Uuid, name, content); err != nil {
		return err
	}
	return d.storage.fs.Devices.RolloverFiles(d.Uuid, storage.StatesPrefix, d.storage.maxStates)
}

func (d Device) GetAppsFilePath(file string) string {
	if d.IsProd {
		return d.storage.fs.Updates.Prod.Apps.FilePath(d.Tag, d.UpdateName, file)
	} else {
		return d.storage.fs.Updates.Ci.Apps.FilePath(d.Tag, d.UpdateName, file)
	}
}

func (d Device) SaveAppliedConfigs(cfg AppliedConfigs) error {
	bytes, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return d.storage.fs.Devices.WriteFile(d.Uuid, storage.ConfigAppliedFile, string(bytes))
}

func (d Device) GetOstreeFilePath(file string) string {
	if d.IsProd {
		return d.storage.fs.Updates.Prod.Ostree.FilePath(d.Tag, d.UpdateName, file)
	} else {
		return d.storage.fs.Updates.Ci.Ostree.FilePath(d.Tag, d.UpdateName, file)
	}
}

func (d Device) GetTufMeta(tag, file string) (string, error) {
	if d.IsProd {
		return d.storage.fs.Updates.Prod.Tuf.ReadFile(tag, d.UpdateName, file)
	} else {
		return d.storage.fs.Updates.Ci.Tuf.ReadFile(tag, d.UpdateName, file)
	}
}

func (d Device) GetConfigs() (configs [3]*storage.ConfigFileSet, timestamp int64, err error) {
	// Returns 3 configs (in order): factory, group, device; and their latest modification timestamp.
	var history []*storage.ConfigFileSet
	timestamp = d.groupNameModifiedAt
	loadConfig := func(history []*storage.ConfigFileSet) *storage.ConfigFileSet {
		if len(history) == 0 {
			return nil
		}
		cfg := history[0]
		if cfg.CreatedAt > timestamp {
			timestamp = cfg.CreatedAt
		}
		return cfg
	}

	if history, err = d.storage.fs.Configs.ReadFactoryConfigHistory(1, true); err != nil {
		return
	} else {
		configs[0] = loadConfig(history)
	}
	if len(d.GroupName) > 0 {
		if history, err = d.storage.fs.Configs.ReadGroupConfigHistory(d.GroupName, 1, true); err != nil {
			return
		} else {
			configs[1] = loadConfig(history)
		}
	}
	if history, err = d.storage.fs.Configs.ReadDeviceConfigHistory(d.Uuid, 1, true); err != nil {
		return
	} else {
		configs[2] = loadConfig(history)
	}
	return
}

func NewStorage(db *storage.DbHandle, fs *storage.FsHandle) (*Storage, error) {
	handle := Storage{
		db:        db,
		fs:        fs,
		maxEvents: 20,
		maxStates: 10,
	}

	if err := db.InitStmt(
		&handle.stmtDeviceCheckIn,
		&handle.stmtDeviceCreate,
		&handle.stmtDeviceGet,
	); err != nil {
		return nil, err
	}

	return &handle, nil
}

func (s Storage) DeviceCreate(uuid, pubkey string, isProd bool) (*Device, error) {
	now := time.Now().Unix()
	if err := s.stmtDeviceCreate.run(uuid, pubkey, now, now, isProd); err != nil {
		return nil, err
	}

	d := Device{
		storage: s,
		Uuid:    uuid,

		Deleted:  false,
		LastSeen: now,
		PubKey:   pubkey,
		IsProd:   isProd,
	}
	return &d, nil
}

func (s Storage) DeviceGet(uuid string) (*Device, error) {
	d := Device{storage: s, Uuid: uuid}
	if err := s.stmtDeviceGet.run(uuid, &d); err != nil {
		if err == sql.ErrNoRows {
			err = nil
		}
		return nil, err
	}
	return &d, nil
}

type stmtDeviceCheckIn storage.DbStmt

func (s *stmtDeviceCheckIn) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("DeviceCheckIn", `
		UPDATE devices
		SET target_name=?, tag=?, ostree_hash=?, apps=?, last_seen=?
		WHERE uuid = ?`,
	)
	return
}

func (s *stmtDeviceCheckIn) run(uuid, targetName, tag, ostreeHash, apps string, lastSeen int64) error {
	_, err := s.Stmt.Exec(targetName, tag, ostreeHash, apps, lastSeen, uuid)
	return err
}

type stmtDeviceCreate storage.DbStmt

func (s *stmtDeviceCreate) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("DeviceCreate", `
		INSERT INTO devices(uuid, pubkey, created_at, last_seen, is_prod, deleted)
		VALUES (?, ?, ?, ?, ?, false)`,
	)
	return
}

func (s *stmtDeviceCreate) run(uuid, pubkey string, createdAt, lastSeen int64, isProd bool) error {
	_, err := s.Stmt.Exec(uuid, pubkey, createdAt, lastSeen, isProd)
	return err
}

type stmtDeviceGet storage.DbStmt

func (s *stmtDeviceGet) Init(db storage.DbHandle) (err error) {
	s.Stmt, err = db.Prepare("DeviceGet", `
		SELECT
			deleted, pubkey, group_name, update_name, last_seen, is_prod, tag, target_name,
			ostree_hash, apps, group_name_modified_at
		FROM devices
		WHERE uuid = ?`,
	)
	return
}

func (s *stmtDeviceGet) run(uuid string, d *Device) error {
	return s.Stmt.QueryRow(uuid).Scan(
		&d.Deleted, &d.PubKey, &d.GroupName, &d.UpdateName, &d.LastSeen, &d.IsProd, &d.Tag, &d.TargetName,
		&d.OstreeHash, &d.Apps, &d.groupNameModifiedAt)
}
