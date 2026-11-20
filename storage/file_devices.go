// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

// Device UUIDs also serve as directory names.
var ValidDeviceUUID = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString

type DevicesFsHandle struct {
	baseFsHandle
}

func (s DevicesFsHandle) Delete(uuid string) error {
	h, err := s.deviceLocalHandle(uuid, false)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(h.root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("error deleting file storage for device %s: %w", uuid, err)
	}
	return nil
}

func (s DevicesFsHandle) ReadFileStream(uuid string, name string) (io.ReadCloser, error) {
	h, err := s.deviceLocalHandle(uuid, false)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(h.root, name)
	return os.Open(path)
}

func (s DevicesFsHandle) ReadFile(uuid, name string) (string, error) {
	h, err := s.deviceLocalHandle(uuid, false)
	if err != nil {
		return "", err
	}
	content, err := h.readFile(name, true)
	if err != nil {
		err = fmt.Errorf("unexpected error reading file %s for device %s: %w", name, uuid, err)
	}
	return content, err
}

func (s DevicesFsHandle) WriteFile(uuid, name, content string) error {
	if h, err := s.deviceLocalHandle(uuid, true); err != nil {
		return err
	} else if err = h.writeFile(name, content, defaultFileAccess); err != nil {
		return fmt.Errorf("error writing file %s for device %s: %w", name, uuid, err)
	}
	return nil
}

func (s DevicesFsHandle) WriteFileStream(uuid, name string, src io.Reader) error {
	if h, err := s.deviceLocalHandle(uuid, true); err != nil {
		return err
	} else {
		if err := h.writeFileStream(name, src, 0o644); err != nil {
			return fmt.Errorf("error writing file stream %s for device %s: %w", name, uuid, err)
		}
	}
	return nil
}

func (s DevicesFsHandle) AppendFile(uuid, name, content string) error {
	if h, err := s.deviceLocalHandle(uuid, true); err != nil {
		return err
	} else if err = h.appendFile(name, content, defaultFileAccess); err != nil {
		return fmt.Errorf("error writing file %s for device %s: %w", name, uuid, err)
	}
	return nil
}

func (s DevicesFsHandle) ListFiles(uuid, prefix string, sortByModTime bool) ([]string, error) {
	h, err := s.deviceLocalHandle(uuid, false)
	if err != nil {
		return nil, err
	}
	names, err := h.matchFiles(prefix, sortByModTime)
	if err != nil {
		err = fmt.Errorf("error listing %s files for device %s: %w", prefix, uuid, err)
	}
	return names, err
}

func (s DevicesFsHandle) RolloverFiles(uuid, prefix string, max int) error {
	if h, err := s.deviceLocalHandle(uuid, true); err != nil {
		return err
	} else if err = h.rolloverFiles(prefix, max); err != nil {
		return fmt.Errorf("error rolling over %s files for device %s: %w", prefix, uuid, err)
	}
	return nil
}

func (s DevicesFsHandle) deviceLocalHandle(uuid string, forUpdate bool) (h baseFsHandle, err error) {
	if !ValidDeviceUUID(uuid) {
		return h, fmt.Errorf("invalid device UUID: %q", uuid)
	}
	h.root = filepath.Join(s.root, uuid)
	if forUpdate {
		if err = h.mkdirs(defaultDirAccess, true); err != nil {
			err = fmt.Errorf("unable to create file storage for device %s: %w", uuid, err)
		}
	}
	return
}
