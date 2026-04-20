// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/foundriesio/dg-satellite/clock"
)

// Outer directory structure:
// - $root/factory/ - factory configs
// - $root/group/$name/ - group configs
// - $root/device/$uuid/ - device configs
// Inner directory structure:
// - .journal - an ordered append-only journal of config history, where the last line is the latest config file name.
// - $config_sha156 - each config file contains the entire config JSON, with file name being a sha256 hash of its contents.
// Note: a config file can be technicallt anything; using a sha256 hash as a name simply allows to avoid collisions.
// An interesting aspect is that any config rollbacks will result into the same hash, effectively compressing disk usage.

type ErrConfigUploadBroken struct {
	err         error
	UploadPath  string
	ConfigsPath string
}

func (e ErrConfigUploadBroken) Error() string {
	return e.err.Error()
}

type ConfigsFsHandle struct {
	baseFsHandle
}

func (s ConfigsFsHandle) ReadFactoryConfig() (content string, timestamp int64, err error) {
	h, _ := s.factoryLocalHandle(false)
	content, timestamp, err = h.readConfig()
	if err != nil {
		err = fmt.Errorf("unexpected error reading factory config: %w", err)
	}
	return
}

func (s ConfigsFsHandle) ReadFactoryConfigHistory(latest int) (contents []string, err error) {
	h, _ := s.factoryLocalHandle(false)
	contents, err = h.readHistory(latest)
	if err != nil {
		err = fmt.Errorf("unexpected error reading factory config history: %w", err)
	}
	return
}

func (s ConfigsFsHandle) WriteFactoryConfig(content string) error {
	if h, err := s.factoryLocalHandle(true); err != nil {
		return err
	} else if err = h.writeConfig(content); err != nil {
		return fmt.Errorf("unexpected error writing factory config: %w", err)
	}
	return nil
}

func (s ConfigsFsHandle) PurgeFactoryConfigHistory(keepLatest int) error {
	if h, err := s.factoryLocalHandle(true); err != nil {
		return err
	} else if err = h.purgeHistory(keepLatest); err != nil {
		return fmt.Errorf("unexpected error purging factory config history: %w", err)
	}
	return nil
}

func (s ConfigsFsHandle) ReadGroupNames() ([]string, error) {
	if entries, err := os.ReadDir(filepath.Join(s.root, ConfigsGroupDir)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, err
	} else {
		// There should be only dir names, but make a sanity check just in case.
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if e.IsDir() {
				names = append(names, e.Name())
			}
		}
		return names, nil
	}
}

func (s ConfigsFsHandle) ReadGroupConfig(name string) (content string, timestamp int64, err error) {
	h, _ := s.groupLocalHandle(name, false)
	content, timestamp, err = h.readConfig()
	if err != nil {
		err = fmt.Errorf("unexpected error reading group config for %s: %w", name, err)
	}
	return
}

func (s ConfigsFsHandle) ReadGroupConfigHistory(name string, latest int) (contents []string, err error) {
	h, _ := s.groupLocalHandle(name, false)
	contents, err = h.readHistory(latest)
	if err != nil {
		err = fmt.Errorf("unexpected error reading group config history for %s: %w", name, err)
	}
	return
}

func (s ConfigsFsHandle) WriteGroupConfig(name, content string) error {
	if h, err := s.groupLocalHandle(name, true); err != nil {
		return err
	} else if err = h.writeConfig(content); err != nil {
		return fmt.Errorf("unexpected error writing group config for %s: %w", name, err)
	}
	return nil
}

func (s ConfigsFsHandle) PurgeGroupConfigHistory(name string, keepLatest int) error {
	if h, err := s.groupLocalHandle(name, true); err != nil {
		return err
	} else if err = h.purgeHistory(keepLatest); err != nil {
		return fmt.Errorf("unexpected error purging group config history for %s: %w", name, err)
	}
	return nil
}

func (s ConfigsFsHandle) ReadDeviceConfig(uuid string) (content string, timestamp int64, err error) {
	h, _ := s.deviceLocalHandle(uuid, false)
	content, timestamp, err = h.readConfig()
	if err != nil {
		err = fmt.Errorf("unexpected error reading device config for %s: %w", uuid, err)
	}
	return
}

func (s ConfigsFsHandle) ReadDeviceConfigHistory(uuid string, latest int) (contents []string, err error) {
	h, _ := s.deviceLocalHandle(uuid, false)
	contents, err = h.readHistory(latest)
	if err != nil {
		err = fmt.Errorf("unexpected error reading device config history for %s: %w", uuid, err)
	}
	return
}

func (s ConfigsFsHandle) WriteDeviceConfig(uuid, content string) error {
	if h, err := s.deviceLocalHandle(uuid, true); err != nil {
		return err
	} else if err = h.writeConfig(content); err != nil {
		return fmt.Errorf("unexpected error writing device config for %s: %w", uuid, err)
	}
	return nil
}

func (s ConfigsFsHandle) SaveUpload(payload io.Reader, onCleanupFailure func(error)) error {
	txDir := ".configs-upload-" + rand.Text()[:10]
	root, destDir := filepath.Split(s.root)
	h := tarFsHandle{root: root}
	return h.unpackTar(payload, destDir,
		TarUnpackReplaceDest(true),
		TarUnpackUseTmpFile("configs.tar"),
		TarUnpackUseTmpDir(txDir),
		TarUnpackOnEvents(tarUnpackEvents{
			onTmpCleanupError: onCleanupFailure,
			onTmpRenameError: func(err error) (bool, error) {
				return true, ErrConfigUploadBroken{
					err:         fmt.Errorf("failed to make uploaded config active: %s", err),
					ConfigsPath: s.root, // not h.root
					UploadPath:  filepath.Join(h.root, txDir),
				}
			},
		}),
	)
}

func (s ConfigsFsHandle) PurgeDeviceConfigHistory(uuid string, keepLatest int) error {
	if h, err := s.deviceLocalHandle(uuid, true); err != nil {
		return err
	} else if err = h.purgeHistory(keepLatest); err != nil {
		return fmt.Errorf("unexpected error purging device config history for %s: %w", uuid, err)
	}
	return nil
}

func (s ConfigsFsHandle) factoryLocalHandle(forUpdate bool) (h configsFsHandle, err error) {
	h.root = filepath.Join(s.root, ConfigsFactoryDir)
	if forUpdate {
		if err = h.mkdirs(defaultDirAccess, true); err != nil {
			err = fmt.Errorf("unable to create file storage for factory config: %w", err)
		}
	}
	return
}

func (s ConfigsFsHandle) groupLocalHandle(name string, forUpdate bool) (h configsFsHandle, err error) {
	h.root = filepath.Join(s.root, ConfigsGroupDir, name)
	if forUpdate {
		if err = h.mkdirs(defaultDirAccess, true); err != nil {
			err = fmt.Errorf("unable to create file storage for group config %s: %w", name, err)
		}
	}
	return
}

func (s ConfigsFsHandle) deviceLocalHandle(uuid string, forUpdate bool) (h configsFsHandle, err error) {
	h.root = filepath.Join(s.root, ConfigsDeviceDir, uuid)
	if forUpdate {
		if err = h.mkdirs(defaultDirAccess, true); err != nil {
			err = fmt.Errorf("unable to create file storage for device config %s: %w", uuid, err)
		}
	}
	return
}

type configsFsHandle struct {
	baseFsHandle
}

type configJournalItem struct {
	name      string
	timestamp int64
}

func (s configsFsHandle) readConfig() (content string, timestamp int64, err error) {
	var journal []configJournalItem
	if journal, err = s.readJournal(); err == nil && len(journal) > 0 {
		latest := journal[len(journal)-1]
		if content, err = s.readFile(latest.name, false); err != nil {
			err = fmt.Errorf("failed to read config file %s: %w", latest.name, err)
		} else {
			timestamp = latest.timestamp
		}
	}
	return
}

func (s configsFsHandle) writeConfig(content string) error {
	// A file based 2-phase commit: write to file and then to journal.
	// If either write fails - config write operation is considered as failed.
	// Any orphan config files will be eventually cleaned by purgeHistory.
	name := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
	if err := s.writeFile(name, content, defaultFileAccess); err != nil {
		return fmt.Errorf("failed to save config file %s: %w", name, err)
	}
	line := fmt.Sprintf("%s:%x\n", name, clock.Now().Unix())
	if err := s.appendFile(ConfigsJournalFile, line, defaultFileAccess); err != nil {
		_ = s.deleteFile(name, true) // Silence cleanup errors - nothing we can do here.
		return fmt.Errorf("failed to write journal for config file %s: %w", name, err)
	}
	return nil
}

func (s configsFsHandle) readHistory(latest int) ([]string, error) {
	names, err := s.readJournalNames()
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, nil
	}
	if len(names) > latest {
		names = names[len(names)-latest:]
	}
	slices.Reverse(names) // Return the latest config as the first item.
	configs := make([]string, 0, len(names))
	for _, name := range names {
		if content, err := s.readFile(name, false); err != nil {
			return nil, fmt.Errorf("failed to read config file %s: %w", name, err)
		} else {
			configs = append(configs, content)
		}
	}
	return configs, nil
}

func (s configsFsHandle) purgeHistory(keepLatest int) (err error) {
	// Only files on disk are purged, not the journal file.
	// That's fine, as the journal file size is minimal.
	// This allows preserving a kind of atomicity of the append-only journal file.
	var keepNames, haveNames []string
	if haveNames, err = s.matchFiles("", false); err != nil {
		return fmt.Errorf("failed to read file list from file system: %w", err)
	}
	if len(haveNames) <= keepLatest {
		return
	}
	if keepNames, err = s.readJournalNames(); err != nil {
		return
	}
	if len(keepNames) > keepLatest {
		keepNames = keepNames[len(keepNames)-keepLatest:]
	}
	for _, name := range haveNames {
		if name != ConfigsJournalFile && !slices.Contains(keepNames, name) {
			if err = s.deleteFile(name, true); err != nil {
				break
			}
		}
	}
	return
}

func (s configsFsHandle) readJournal() ([]configJournalItem, error) {
	var items []configJournalItem
	for line, err := range s.readFileLines(ConfigsJournalFile, true, nil) {
		if err != nil {
			return nil, fmt.Errorf("failed to read journal file: %w", err)
		}
		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("failed to parse journal item %s: wrong format", line)
		}
		ts, err := strconv.ParseInt(parts[1], 16, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse journal item %s: wrong format", line)
		}
		items = append(items, configJournalItem{parts[0], ts})
	}
	return items, nil
}

func (s configsFsHandle) readJournalNames() ([]string, error) {
	if items, err := s.readJournal(); err != nil {
		return nil, err
	} else {
		names := make([]string, len(items))
		for idx, item := range items {
			names[idx] = item.name
		}
		return names, nil
	}
}
