// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
)

// Device UUIDs also serve as directory names.
var ValidDeviceUUID = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString

type DevicesFsHandle struct {
	osRootHandle
}

func (s DevicesFsHandle) Delete(uuid string) error {
	if !ValidDeviceUUID(uuid) {
		return fmt.Errorf("invalid device UUID: %q", uuid)
	}
	if err := s.root.RemoveAll(uuid); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("error deleting file storage for device %s: %w", uuid, err)
	}
	return nil
}

func (s DevicesFsHandle) ReadFileStream(uuid string, name string) (io.ReadCloser, error) {
	var file *os.File
	err := s.withHandle(uuid, false, func(h osRootHandle) error {
		var err error
		file, err = h.root.Open(name)
		return err
	})
	if err != nil {
		return nil, err
	}
	return file, nil
}

func (s DevicesFsHandle) ReadFile(uuid, name string) (string, error) {
	var content string
	err := s.withHandle(uuid, false, func(h osRootHandle) error {
		var err error
		content, err = h.readFile(name, true)
		return err
	})
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	} else if err != nil {
		err = fmt.Errorf("unexpected error reading file %s for device %s: %w", name, uuid, err)
	}
	return content, err
}

func (s DevicesFsHandle) WriteFile(uuid, name, content string) error {
	return s.withHandle(uuid, true, func(h osRootHandle) error {
		if err := h.writeFile(name, content, defaultFileAccess); err != nil {
			return fmt.Errorf("error writing file %s for device %s: %w", name, uuid, err)
		}
		return nil
	})
}

func (s DevicesFsHandle) WriteFileStream(uuid, name string, src io.Reader) error {
	return s.withHandle(uuid, true, func(h osRootHandle) error {
		if err := h.writeFileStream(name, src, 0o644); err != nil {
			return fmt.Errorf("error writing file stream %s for device %s: %w", name, uuid, err)
		}
		return nil
	})
}

func (s DevicesFsHandle) AppendFile(uuid, name, content string) error {
	return s.withHandle(uuid, true, func(h osRootHandle) error {
		if err := h.appendFile(name, content, defaultFileAccess); err != nil {
			return fmt.Errorf("error writing file %s for device %s: %w", name, uuid, err)
		}
		return nil
	})
}

func (s DevicesFsHandle) ListFiles(uuid, prefix string, sortByModTime bool) ([]string, error) {
	var names []string
	err := s.withHandle(uuid, false, func(h osRootHandle) error {
		var err error
		names, err = h.matchFiles(prefix, sortByModTime)
		return err
	})
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	} else if err != nil {
		err = fmt.Errorf("error listing %s files for device %s: %w", prefix, uuid, err)
	}
	return names, err
}

func (s DevicesFsHandle) RolloverFiles(uuid, prefix string, max int) error {
	return s.withHandle(uuid, true, func(h osRootHandle) error {
		if err := h.rolloverFiles(prefix, max); err != nil {
			return fmt.Errorf("error rolling over %s files for device %s: %w", prefix, uuid, err)
		}
		return nil
	})
}

func (s DevicesFsHandle) withHandle(uuid string, forUpdate bool, fn func(osRootHandle) error) error {
	if !ValidDeviceUUID(uuid) {
		return fmt.Errorf("invalid device UUID: %q", uuid)
	}
	if forUpdate {
		if err := s.root.MkdirAll(uuid, defaultDirAccess); err != nil {
			return fmt.Errorf("unable to create file storage for device %s: %w", uuid, err)
		}
	}
	root, err := s.root.OpenRoot(uuid)
	if err != nil {
		return err
	}
	h := osRootHandle{root: root}
	defer h.Close()
	return fn(h)
}
