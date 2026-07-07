// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/tuf"
)

// rootJsonSuffix is the suffix ota-tuf uses for root metadata files (e.g.
// "1.root.json").
const rootJsonSuffix = ".root.json"

// tufKeyFileSuffix is the suffix fioctl/garage-sign use for private key files
// stored in the offline keys tarball.
const tufKeyFileSuffix = ".sec"

type TufInitCmd struct {
	ImportKeys  string `arg:"--import-keys" help:"Path to a fioctl offline keys tarball with the root key(s) to sign the rotation; enables TUF root import (requires auth-init to have been run first)"`
	ImportRoots string `arg:"--import-roots" help:"Path to a gzipped tarball containing all root.json files to import"`
}

func (c TufInitCmd) Run(args CommonArgs) error {
	fs, err := storage.NewFs(args.DataDir)
	if err != nil {
		return err
	}
	if c.ImportKeys != "" || c.ImportRoots != "" {
		if c.ImportKeys == "" {
			return fmt.Errorf("--import-keys is required to import TUF root metadata")
		}
		if c.ImportRoots == "" {
			return fmt.Errorf("--import-roots is required to import TUF root metadata")
		}
		roots, err := loadTufRootsArchive(c.ImportRoots)
		if err != nil {
			return err
		}
		keys, err := loadTufKeysArchive(c.ImportKeys)
		if err != nil {
			return err
		}
		return fs.Tuf.ImportTuf(roots, keys)
	}
	return fs.Tuf.InitTuf()
}

// loadTufKeysArchive opens a fioctl offline keys tarball and extracts the
// root key files it contains. Only private key files (those ending in
// ".sec") are parsed; all other archive entries are ignored.
func loadTufKeysArchive(path string) ([]tuf.AtsKey, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("unable to open keys archive: %w", err)
	}
	defer f.Close() // nolint:errcheck

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("unable to open keys archive (expected a gzipped tarball): %w", err)
	}
	defer gz.Close() // nolint:errcheck

	var keys []tuf.AtsKey
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("unable to read keys archive: %w", err)
		}
		if hdr.Typeflag == tar.TypeDir || !strings.HasSuffix(hdr.Name, tufKeyFileSuffix) {
			continue
		}

		if data, err := io.ReadAll(tr); err != nil {
			return nil, fmt.Errorf("unable to read %s from keys archive: %w", hdr.Name, err)
		} else {
			var key tuf.AtsKey
			if err := json.Unmarshal(data, &key); err != nil {
				return nil, fmt.Errorf("unable to parse key file %s: %w", hdr.Name, err)
			}
			if key.KeyType != "" && key.KeyValue.Private != "" {
				keys = append(keys, key)
			}
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("keys archive does not contain any valid private key files (expected .sec files with private key material)")
	}
	return keys, nil
}

// loadTufRootsArchive reads a gzipped tar archive and returns the raw bytes of
// every root.json file it contains. Only files ending in ".root.json" are
// extracted; all other archive entries are ignored.
func loadTufRootsArchive(path string) ([][]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("unable to open roots archive: %w", err)
	}
	defer f.Close() // nolint:errcheck

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("unable to open roots archive (expected a gzipped tarball): %w", err)
	}
	defer gz.Close() // nolint:errcheck

	var roots [][]byte
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("unable to read roots archive: %w", err)
		}
		if hdr.Typeflag == tar.TypeDir || !strings.HasSuffix(hdr.Name, rootJsonSuffix) {
			continue
		}

		if data, err := io.ReadAll(tr); err != nil {
			return nil, fmt.Errorf("unable to read %s from roots archive: %w", hdr.Name, err)
		} else {
			roots = append(roots, data)
		}
	}
	if len(roots) == 0 {
		return nil, fmt.Errorf("roots archive does not contain any root.json files (expected files ending in %q)", rootJsonSuffix)
	}
	return roots, nil
}
