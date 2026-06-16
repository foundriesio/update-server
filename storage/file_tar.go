// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type tarUnpackConfig struct {
	createDest  bool
	replaceDest bool
	mergeDest   bool
	dirAccess   os.FileMode
	fileAccess  os.FileMode
	tmpFile     string
	tmpDir      string
	events      tarUnpackEvents
}

type tarUnpackEvents struct {
	// Notifications that can be used to interrupt or amend unpacking process
	onUnpackStarted  func() error
	onTarHeaderSeen  func(*TarHeader) (skip bool, err error)
	onUnpackComplete func() error
	// Called after unpacking and contents are moved to final destination.
	// If it returns an error, the unpacked files are cleaned up.
	onCommit func() error
	// Error handlers for two specific errors of interest
	onTmpRenameError  func(error) (isDestCorrupted bool, err error)
	onTmpCleanupError func(error)
}

type (
	TarUnpackOption func(tarUnpackConfig) tarUnpackConfig

	TarHeader = tar.Header
)

func TarUnpackDirAccess(mode os.FileMode) TarUnpackOption {
	return func(cfg tarUnpackConfig) tarUnpackConfig {
		cfg.dirAccess = mode
		return cfg
	}
}

func TarUnpackFileAccess(mode os.FileMode) TarUnpackOption {
	return func(cfg tarUnpackConfig) tarUnpackConfig {
		cfg.fileAccess = mode
		return cfg
	}
}

func TarUnpackCreateDest(val bool) TarUnpackOption {
	return func(cfg tarUnpackConfig) tarUnpackConfig {
		cfg.createDest = val
		return cfg
	}
}

func TarUnpackMergeDest(val bool) TarUnpackOption {
	return func(cfg tarUnpackConfig) tarUnpackConfig {
		cfg.mergeDest = val
		return cfg
	}
}

func TarUnpackReplaceDest(val bool) TarUnpackOption {
	return func(cfg tarUnpackConfig) tarUnpackConfig {
		cfg.replaceDest = val
		return cfg
	}
}

// Use temporary directory to unpack files instead of unpacking directory into the destination.
// A temporary directory is then moved to destination in a two-phase commit.
func TarUnpackUseTmpDir(val string) TarUnpackOption {
	return func(cfg tarUnpackConfig) tarUnpackConfig {
		cfg.tmpDir = val
		return cfg
	}
}

// Use temporary file for a tarball instead of processing it in memory.
// This allows to read the file from network faster, minimizing a chance for errors and network stack conservation.
func TarUnpackUseTmpFile(val string) TarUnpackOption {
	return func(cfg tarUnpackConfig) tarUnpackConfig {
		cfg.tmpFile = val
		return cfg
	}
}

func TarUnpackOnEvents(val tarUnpackEvents) TarUnpackOption {
	return func(cfg tarUnpackConfig) tarUnpackConfig {
		cfg.events = val
		return cfg
	}
}

type tarFsHandle baseFsHandle

func (s tarFsHandle) unpackTar(srcReader io.Reader, destDir string, opts ...TarUnpackOption) error {
	cfg := tarUnpackConfig{
		createDest:  true,
		mergeDest:   false,
		replaceDest: false,
		dirAccess:   defaultDirAccess,
		fileAccess:  defaultFileAccess,
	}
	for _, opt := range opts {
		cfg = opt(cfg)
	}

	// A filepath.Join warrants that destDirPath is clean, so that we can use absPathNoEscape freely below.
	destDirPath := filepath.Join(s.root, destDir)
	if err := s._checkDestDir(destDir, destDirPath, cfg); err != nil {
		return err
	}

	unpacker := func(destDirPath string) error {
		if len(cfg.tmpFile) > 0 {
			if tmpReader, err := s._handleTmpFile(srcReader, cfg); err != nil {
				return err
			} else {
				defer tmpReader.Close() //nolint:errcheck
				srcReader = tmpReader
			}
		}

		return s._unpackTar(srcReader, destDirPath, cfg)
	}

	if len(cfg.tmpDir) > 0 {
		return s._handleTmpDir(destDirPath, cfg, unpacker)
	} else {
		return unpacker(destDirPath)
	}
}

func (s tarFsHandle) _checkDestDir(destDir, destDirPath string, cfg tarUnpackConfig) error {
	var destExists, destEmpty bool
	if destItems, err := os.ReadDir(destDirPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to check if destination '%s' exists: %w", destDir, err)
		}
	} else {
		destExists = true
		destEmpty = len(destItems) == 0
	}

	if !destExists {
		if cfg.createDest {
			if err := os.MkdirAll(destDirPath, cfg.dirAccess); err != nil {
				return fmt.Errorf("failed to create destination '%s': %w", destDir, err)
			}
		}
	} else if !destEmpty {
		if cfg.replaceDest {
			if err := os.RemoveAll(destDirPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("failed to clean destination '%s': %w", destDir, err)
			}
			if err := os.MkdirAll(destDirPath, cfg.dirAccess); err != nil {
				return fmt.Errorf("failed to create destination '%s': %w", destDir, err)
			}
		} else if !cfg.mergeDest {
			return fmt.Errorf("destination '%s' already exists and is not empty", destDir)
		}
	}
	return nil
}

func (s tarFsHandle) _handleTmpDir(destDirPath string, cfg tarUnpackConfig, unpacker func(string) error) error {
	txDirPath := filepath.Join(s.root, cfg.tmpDir)
	unpackDirPath := filepath.Join(txDirPath, "unpacked")
	backupDirPath := filepath.Join(txDirPath, "backup")

	if err := os.MkdirAll(txDirPath, cfg.dirAccess); err != nil {
		return fmt.Errorf("failed to create a temporary directory: %w", err)
	}
	var isDestCorrupted bool
	defer func() {
		if !isDestCorrupted {
			if err := os.RemoveAll(txDirPath); err != nil && cfg.events.onTmpCleanupError != nil {
				cfg.events.onTmpCleanupError(err)
			}
		}
	}()

	if cfg.mergeDest {
		if err := os.CopyFS(unpackDirPath, os.DirFS(destDirPath)); err != nil {
			return fmt.Errorf("failed to copy original directory files for merging: %w", err)
		}
	}
	if err := unpacker(unpackDirPath); err != nil {
		return err
	}

	// Two-phase commit below: move current directory to backup, and then new directory to current directory.
	// Both operations are atomic on Linux; but there is a tiny chance to fail in the middle.
	// See comments below if/when/how this is recoverable.
	if err := os.Rename(destDirPath, backupDirPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		// Linux warrants that the rename is atomic i.e. if it failed - there was no rename.
		// Source directory is intact here... unless there was a hard power failure, in which case this process is dead too.
		return fmt.Errorf("failed to backup existing directory: %w", err)
	}
	if err := os.Rename(unpackDirPath, destDirPath); err != nil {
		// If the second phase fails, source directory is moved to backup, while unpacked directory is not yet moved.
		// Although the destination directory does not exist in this case - this is a potential data corruption.
		// Let the caller decide what to do in this case. By default do nothing.
		if cfg.events.onTmpRenameError != nil {
			isDestCorrupted, err = cfg.events.onTmpRenameError(err)
		} else {
			err = fmt.Errorf("failed to rename unpacked directory: %w", err)
		}
		return err
	}
	if cfg.events.onCommit != nil {
		if err := cfg.events.onCommit(); err != nil {
			if rmErr := os.RemoveAll(destDirPath); rmErr != nil && cfg.events.onTmpCleanupError != nil {
				cfg.events.onTmpCleanupError(rmErr)
			}
			return err
		}
	}
	return nil
}

func (s tarFsHandle) _handleTmpFile(srcReader io.Reader, cfg tarUnpackConfig) (io.ReadCloser, error) {
	tmpFilePath := filepath.Join(s.root, cfg.tmpDir, cfg.tmpFile)
	// Need a tarball to close before processing it; thus wrap this into a function.
	if err := func() (err error) {
		var file io.WriteCloser
		if file, err = os.OpenFile(tmpFilePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, cfg.fileAccess); err != nil {
			return
		}
		defer func() {
			// Close error here mean that the write finalization failed (e.g. a failure flushing to disk)
			if err2 := file.Close(); err2 != nil && err == nil {
				err = err2
			}
		}()
		if _, err = io.Copy(file, srcReader); err != nil {
			err = fmt.Errorf("failed to save tarball to '%s': %w", cfg.tmpFile, err)
		}
		return
	}(); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(tmpFilePath, os.O_RDONLY, 0)
	if err != nil {
		err = fmt.Errorf("failed to read tarball from '%s': %w", cfg.tmpFile, err)
	}
	return file, err
}

func (s tarFsHandle) _unpackTar(srcReader io.Reader, destDirPath string, cfg tarUnpackConfig) error {
	if cfg.events.onUnpackStarted != nil {
		if err := cfg.events.onUnpackStarted(); err != nil {
			return err
		}
	}
	// Unpack config upload tarball; if it fails - halt.
	tarReader := tar.NewReader(srcReader)
	for {
		hdr, err := tarReader.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break // Success
			}
			return fmt.Errorf("failed to unpack tarball header: %w", err)
		}
		if cfg.events.onTarHeaderSeen != nil {
			if skip, err := cfg.events.onTarHeaderSeen(hdr); err != nil {
				return err
			} else if skip {
				continue
			}
		}
		switch hdr.Typeflag {
		case tar.TypeReg:
			// Logic continues after the switch.
		case tar.TypeDir:
			dirPath, err := AbsPathNoEscape(destDirPath, hdr.Name)
			if err == nil {
				err = os.MkdirAll(dirPath, cfg.dirAccess)
			}
			if err != nil {
				return fmt.Errorf("failed to unpack directory '%s': %w", hdr.Name, err)
			}
			continue
		default:
			return fmt.Errorf("failed to unpack file '%s': unsupported file type %d", hdr.Name, hdr.Typeflag)
		}
		if len(hdr.Name) == 0 {
			return errors.New("failed to unpack file with empty name")
		}
		filePath, err := AbsPathNoEscape(destDirPath, hdr.Name)
		if err != nil {
			return fmt.Errorf("failed to unpack file '%s': %w", hdr.Name, err)
		}
		dirPath := filepath.Dir(filePath)
		if err = os.MkdirAll(dirPath, cfg.dirAccess); err != nil {
			return fmt.Errorf("failed to unpack file '%s': %w", hdr.Name, err)
		}
		if file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY, cfg.fileAccess); err != nil {
			return fmt.Errorf("failed to unpack file '%s': %w", hdr.Name, err)
		} else if _, err = io.Copy(file, tarReader); err != nil {
			return fmt.Errorf("failed to unpack file '%s': %w", hdr.Name, err)
		}
	}
	if cfg.events.onUnpackComplete != nil {
		if err := cfg.events.onUnpackComplete(); err != nil {
			return err
		}
	}
	return nil
}

func AbsPathNoEscape(root, path string) (absPath string, err error) {
	// Assume that the root is clean (a caller is responsible for this for performance optimization).
	// A filepath.Join warrants that the absPath is also clean.
	// So, in order to check for the root path escape attempt, we only need to check for the path prefix.
	absPath = filepath.Join(root, path)
	var isEscaping bool
	if len(root) >= len(absPath) {
		// Trivial case: path is not longer than root, so it must equal root.
		isEscaping = root != absPath
	} else if root != "/" {
		// It is not possible to escape outside the top directory.
		// In all other cases filepath.Clean warrants that root does not end in a slash.
		isEscaping = !strings.HasPrefix(absPath, root+string(filepath.Separator))
	}
	if isEscaping {
		err = errors.New("directory escape attempt")
	}
	return

}
