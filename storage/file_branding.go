// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// BrandingDir is the directory operators drop branding.json and assets into.
func (c FsConfig) BrandingDir() string {
	return filepath.Join(string(c), BrandingDir)
}

// ReadBrandingConfig returns the raw bytes of branding.json, or (nil, nil) when
// the file does not exist — an absent file means "use built-in defaults".
func (c FsConfig) ReadBrandingConfig() ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(c.BrandingDir(), BrandingConfigFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return data, err
}
