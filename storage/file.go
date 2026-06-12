// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"
)

const (
	// Global files/dirs
	AuditDir   = "audit"
	AuthDir    = "auth"
	CertsDir   = "certs"
	ConfigsDir = "configs"
	DbFile     = "db.sqlite"
	DevicesDir = "devices"
	UpdatesDir = "updates"

	partialFileSuffix  = "..part"
	rolloutJournalFile = "rollouts.journal"

	CertsCasPemFile = "cas.pem"
	CertsTlsCsrFile = "tls.csr"
	CertsTlsKeyFile = "tls.key"
	CertsTlsPemFile = "tls.pem"

	AuthConfigFile = "auth-config.json"
	HmacFile       = "hmac.secret"

	// Per config class files/dirs
	ConfigsFactoryDir  = "factory"
	ConfigsGroupDir    = "group"
	ConfigsDeviceDir   = "device"
	ConfigsJournalFile = ".journal"
	ConfigSotaOverride = "z-50-fioctl.toml"

	// Per device files/dirs
	ConfigAppliedFile   = "config-applied"
	AktomlFile          = "aktoml"
	HwInfoFile          = "hardware-info"
	NetInfoFile         = "network-info"
	EventsPrefix        = "events"
	StatesPrefix        = "apps-states"
	TestsPrefix         = "tests"
	TestArtifactsPrefix = "test-artifacts"

	// Per update files/dirs
	// Update categories
	UpdatesTufDir      = "tuf"
	UpdatesOstreeDir   = "ostree_repo"
	UpdatesAppsDir     = "apps"
	UpdatesRolloutsDir = "rollouts"
	UpdatesLogsDir     = "logs"
	// TUF category files
	TufRootFile      = "root.json"
	TufTimestampFile = "timestamp.json"
	TufSnapshotFile  = "snapshot.json"
	TufTargetsFile   = "targets.json"
	// Logs category files
	LogRolloutsFile = "rollouts.log"
)

const (
	// File & Dir access
	defaultDirAccess  os.FileMode = 0o750
	defaultFileAccess os.FileMode = 0o640
	secureFileAccess  os.FileMode = 0o600
)

type (
	FsConfig string
	DoneChan = <-chan struct{} // Dictated by Context.Done
)

func (c FsConfig) RootDir() string {
	return string(c)
}

func (c FsConfig) AuditDir() string {
	return filepath.Join(string(c), AuditDir)
}

func (c FsConfig) AuthDir() string {
	return filepath.Join(string(c), AuthDir)
}

func (c FsConfig) DbFile() string {
	return filepath.Join(string(c), DbFile)
}

func (c FsConfig) CertsDir() string {
	return filepath.Join(string(c), CertsDir)
}

func (c FsConfig) DevicesDir() string {
	return filepath.Join(string(c), DevicesDir)
}

func (c FsConfig) ConfigsDir() string {
	return filepath.Join(string(c), ConfigsDir)
}

func (c FsConfig) UpdatesDir() string {
	return filepath.Join(string(c), UpdatesDir)
}

type FsHandle struct {
	Config FsConfig

	Audit   AuditLogsFsHandle
	Auth    AuthFsHandle
	Certs   CertsFsHandle
	Configs ConfigsFsHandle
	Devices DevicesFsHandle
	Updates updatesFsHandleWrap
}

func NewFs(root string) (*FsHandle, error) {
	fs := &FsHandle{Config: FsConfig(root)}
	fs.Audit.root = fs.Config.AuditDir()
	fs.Auth.root = fs.Config.AuthDir()
	fs.Certs.root = fs.Config.CertsDir()
	fs.Configs.root = fs.Config.ConfigsDir()
	fs.Devices.root = fs.Config.DevicesDir()
	fs.Updates.init(fs.Config.UpdatesDir())

	for _, h := range []baseFsHandle{
		fs.Audit.baseFsHandle,
		fs.Auth.baseFsHandle,
		fs.Certs.baseFsHandle,
		fs.Configs.baseFsHandle,
		fs.Devices.baseFsHandle,
		fs.Updates.baseFsHandle,
	} {
		if err := h.mkdirs(defaultDirAccess, true); err != nil {
			return nil, fmt.Errorf("unable to initialize file storage: %w", err)
		}
	}
	return fs, nil
}

type baseFsHandle struct {
	root string
}

func (s baseFsHandle) mkdirs(mode os.FileMode, ignoreExists bool) error { //nolint:unparam
	if ignoreExists {
		return os.MkdirAll(s.root, mode)
	} else {
		return os.Mkdir(s.root, mode)
	}
}

func (s baseFsHandle) readFile(name string, ignoreNotExist bool) (string, error) {
	if content, err := os.ReadFile(filepath.Join(s.root, name)); err == nil {
		return string(content), nil
	} else if ignoreNotExist && errors.Is(err, os.ErrNotExist) {
		return "", nil
	} else {
		return "", err
	}
}

func (s baseFsHandle) readFileLines(name string, ignoreNotExist bool, infinityStop DoneChan) iter.Seq2[string, error] {
	// memory efficient way to read lines from a potentially large file
	return func(yield func(string, error) bool) {
		if fd, err := os.OpenFile(filepath.Join(s.root, name), os.O_RDONLY, 0); err != nil {
			if !ignoreNotExist || !errors.Is(err, os.ErrNotExist) {
				yield("", err)
			}
		} else {
			defer fd.Close() // nolint:errcheck
		TAIL:
			scanner := bufio.NewScanner(fd) // line reader
			for scanner.Scan() {
				if !yield(scanner.Text(), nil) {
					return
				}
			}
			if err = scanner.Err(); err != nil {
				yield("", err)
				return
			}
			if infinityStop != nil {
				// Tail functionality - simply re-create the scanner with the same fd after some time.
				// File position remains the same, so a new scanner continues from it.
				select {
				case <-infinityStop:
					return
				case <-time.After(5 * time.Millisecond):
					goto TAIL
				}
			}
		}
	}
}

func (s baseFsHandle) writeFile(name, content string, mode os.FileMode) error {
	path := filepath.Join(s.root, name)
	partial := filepath.Join(s.root, name+partialFileSuffix)
	if fd, err := os.OpenFile(partial, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode); err != nil {
		return err
	} else if _, err = fd.WriteString(content); err != nil {
		_ = fd.Close()
		return err
	} else if err = fd.Sync(); err != nil {
		_ = fd.Close()
		return err
	} else if err = fd.Close(); err != nil {
		return err
	} else {
		return os.Rename(partial, path)
	}
}

func (s baseFsHandle) writeFileStream(name string, src io.Reader, mode os.FileMode) error {
	path := filepath.Join(s.root, name)
	dst, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err = io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return err
	}
	return dst.Close()
}

func (s baseFsHandle) appendFile(name, content string, mode os.FileMode) error { //nolint:unparam
	// O_APPEND + O_SYNC on Linux warrants that concurrent file appends up to 1MB are serialized.
	fd, err := os.OpenFile(filepath.Join(s.root, name),
		os.O_CREATE|os.O_APPEND|syscall.O_SYNC|os.O_WRONLY, mode)
	if err == nil {
		_, err = fd.Write([]byte(content))
		if err != nil {
			_ = fd.Close()
		} else {
			err = fd.Close()
		}
	}
	return err
}

func (s baseFsHandle) deleteFile(name string, ignoreNotExist bool) error {
	err := os.Remove(filepath.Join(s.root, name))
	if err != nil && ignoreNotExist && errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	return err
}

func (s baseFsHandle) rolloverFiles(prefix string, max int) error {
	names, err := s.matchFiles(prefix, true)
	if err == nil {
		for i := 0; i < len(names)-max; i++ {
			if err = s.deleteFile(names[i], false); err != nil {
				break
			}
		}
	}
	return err
}

func (s baseFsHandle) matchFiles(prefix string, sortByModTime bool) ([]string, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, err
	}
	infos := make([]os.FileInfo, 0, len(entries))
	for _, entry := range entries {
		if info, err := entry.Info(); err != nil {
			return nil, err
		} else {
			name := info.Name()
			if strings.HasSuffix(name, partialFileSuffix) {
				// Filter out partial files - uploads in progress or data corruptions
				continue
			} else if len(prefix) == 0 || strings.HasPrefix(name, prefix) {
				infos = append(infos, info)
			}
		}
	}
	if sortByModTime {
		slices.SortFunc(infos, func(a, b os.FileInfo) int {
			// UnixMilli is int64, but in our universe UnixMilli difference of two events files of the same device is int.
			return int(a.ModTime().UnixMilli() - b.ModTime().UnixMilli())
		})
	}
	names := make([]string, 0, len(infos))
	for _, info := range infos {
		names = append(names, info.Name())
	}
	return names, nil
}
