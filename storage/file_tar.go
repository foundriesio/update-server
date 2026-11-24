// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	// Default limits to control an uncompressed tarball size on disk:
	// - A maximum disk size for all uncompressed files and directories, including filesystem index: 256 MB (configurable).
	// - A maximum disk size for a single uncompressed file entry: 32 MB (configurable).
	// - A constant filesystem index block size, actual for the majority of filesystems: 4 KB (fixed).
	// - A maximum number of file and directory entries to unpack: 262144 (fixed), that gives a total index size 1 GB.
	// - A maximum file path length for a single uncompressed file or directory entry: 1 KB (fixed).
	defaultUnpackedTarDiskSizeLimit  int64 = 1 << 28 // 256 MB
	defaultUnpackedTarItemSizeLimit  int64 = 1 << 25 // 32 MB
	defaultUnpackedTarDiskBlockSize  int64 = 1 << 12 // 4 KB
	defaultUnpackedTarItemCountLimit int   = 1 << 18 // ~262K, gives 1 GB when multiplied by block size.
	defaultUnpackedTarPathLenLimit   int   = 1 << 10 // 1 KB
)

var (
	ErrTarUnpackExceedsLimit           = errors.New("tar unpack exceeds limit")
	ErrTarUnpackExceedsLimitRawSize    = fmt.Errorf("%w of total compressed disk size", ErrTarUnpackExceedsLimit)
	ErrTarUnpackExceedsLimitDiskSize   = fmt.Errorf("%w of total decompressed disk size", ErrTarUnpackExceedsLimit)
	ErrTarUnpackExceedsLimitFileSize   = fmt.Errorf("%w of single decompressed file size", ErrTarUnpackExceedsLimit)
	ErrTarUnpackExceedsLimitFileCount  = fmt.Errorf("%w of total file count", ErrTarUnpackExceedsLimit)
	ErrTarUnpackExceedsLimitPathLength = fmt.Errorf("%w of single file path length", ErrTarUnpackExceedsLimit)
)

type tarUnpackConfig struct {
	createDest  bool
	replaceDest bool
	mergeDest   bool
	dirAccess   os.FileMode
	fileAccess  os.FileMode
	tmpFile     string
	tmpDir      string
	maxItemSize int64
	maxDiskSize int64
	maxNumItems int
	maxPathLen  int
	blockSize   int64
	events      tarUnpackEvents
}

type tarUnpackEvents struct {
	// Notifications that can be used to interrupt or amend unpacking process
	onUnpackStarted  func() error
	onTarHeaderSeen  func(*TarHeader) (skip bool, err error)
	onUnpackComplete func() error
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

func TarUnpackMaxItemFileSize(val int64) TarUnpackOption {
	return func(cfg tarUnpackConfig) tarUnpackConfig {
		cfg.maxItemSize = val
		return cfg
	}
}

func TarUnpackMaxTotalDiskSize(val int64) TarUnpackOption {
	return func(cfg tarUnpackConfig) tarUnpackConfig {
		cfg.maxDiskSize = val
		return cfg
	}
}

func TarUnpackOnEvents(val tarUnpackEvents) TarUnpackOption {
	return func(cfg tarUnpackConfig) tarUnpackConfig {
		cfg.events = val
		return cfg
	}
}

type tarFsHandle struct {
	osRootHandle
}

func (s tarFsHandle) unpackTar(srcReader io.Reader, destDir string, opts ...TarUnpackOption) error {
	cfg := tarUnpackConfig{
		createDest:  true,
		mergeDest:   false,
		replaceDest: false,
		dirAccess:   defaultDirAccess,
		fileAccess:  defaultFileAccess,
		maxItemSize: defaultUnpackedTarItemSizeLimit,
		maxDiskSize: defaultUnpackedTarDiskSizeLimit,
		maxNumItems: defaultUnpackedTarItemCountLimit,
		maxPathLen:  defaultUnpackedTarPathLenLimit,
		blockSize:   defaultUnpackedTarDiskBlockSize,
	}
	for _, opt := range opts {
		cfg = opt(cfg)
	}

	destDirPath := filepath.Clean(destDir)
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
	if destItems, err := fs.ReadDir(s.root.FS(), filepath.ToSlash(destDirPath)); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to check if destination '%s' exists: %w", destDir, err)
		}
	} else {
		destExists = true
		destEmpty = len(destItems) == 0
	}

	switch {
	case !destExists:
		if cfg.createDest {
			if err := s.root.MkdirAll(destDirPath, cfg.dirAccess); err != nil {
				return fmt.Errorf("failed to create destination '%s': %w", destDir, err)
			}
		}
	case !destEmpty:
		if cfg.replaceDest {
			if err := s.root.RemoveAll(destDirPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("failed to clean destination '%s': %w", destDir, err)
			}
			if err := s.root.MkdirAll(destDirPath, cfg.dirAccess); err != nil {
				return fmt.Errorf("failed to create destination '%s': %w", destDir, err)
			}
		} else if !cfg.mergeDest {
			return fmt.Errorf("destination '%s' already exists and is not empty", destDir)
		}
	}
	return nil
}

func (s tarFsHandle) _handleTmpDir(destDirPath string, cfg tarUnpackConfig, unpacker func(string) error) error {
	txDirPath := filepath.Clean(cfg.tmpDir)
	unpackDirPath := filepath.Join(txDirPath, "unpacked")
	backupDirPath := filepath.Join(txDirPath, "backup")

	if err := s.root.MkdirAll(txDirPath, cfg.dirAccess); err != nil {
		return fmt.Errorf("failed to create a temporary directory: %w", err)
	}
	var isDestCorrupted bool
	defer func() {
		if !isDestCorrupted {
			if err := s.root.RemoveAll(txDirPath); err != nil && cfg.events.onTmpCleanupError != nil {
				cfg.events.onTmpCleanupError(err)
			}
		}
	}()

	if cfg.mergeDest {
		if err := s._copyDir(destDirPath, unpackDirPath); err != nil {
			return fmt.Errorf("failed to copy original directory files for merging: %w", err)
		}
	}
	if err := unpacker(unpackDirPath); err != nil {
		return err
	}

	// Two-phase commit below: move current directory to backup, and then new directory to current directory.
	// Both operations are atomic on Linux; but there is a tiny chance to fail in the middle.
	// See comments below if/when/how this is recoverable.
	if err := s.root.Rename(destDirPath, backupDirPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		// Linux warrants that the rename is atomic i.e. if it failed - there was no rename.
		// Source directory is intact here... unless there was a hard power failure, in which case this process is dead too.
		return fmt.Errorf("failed to backup existing directory: %w", err)
	}
	if err := s.root.Rename(unpackDirPath, destDirPath); err != nil {
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
	return nil
}

func (s tarFsHandle) _handleTmpFile(srcReader io.Reader, cfg tarUnpackConfig) (io.ReadCloser, error) {
	// Reuse uncompressed disk size limit, as a temporary pre-fetched tarball is removed after unpacking.
	// Allow 1 byte overhead to check if the limit was hit.
	limitReader := &io.LimitedReader{R: srcReader, N: 1 + cfg.maxDiskSize}
	// Write tarball to a temporary file and close, so that it is flushed to disk.
	tmpFilePath := filepath.Join(cfg.tmpDir, cfg.tmpFile)
	file, err := s.root.OpenFile(tmpFilePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, cfg.fileAccess)
	if err == nil {
		err = _copyAndCloseOut(file, limitReader)
	}
	if err == nil && limitReader.N <= 0 {
		err = fmt.Errorf("%w: %d", ErrTarUnpackExceedsLimitRawSize, cfg.maxDiskSize)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to save tarball to '%s': %w", cfg.tmpFile, err)
	}
	// Read tarball from that temporary file and return descriptor for processing.
	file, err = s.root.OpenFile(tmpFilePath, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to read tarball from '%s': %w", cfg.tmpFile, err)
	}
	return file, nil
}

func (s tarFsHandle) _unpackTar(srcReader io.Reader, destDirPath string, cfg tarUnpackConfig) error {
	if cfg.events.onUnpackStarted != nil {
		if err := cfg.events.onUnpackStarted(); err != nil {
			return err
		}
	}
	if err := s.root.MkdirAll(destDirPath, cfg.dirAccess); err != nil {
		return fmt.Errorf("failed to create unpack directory: %w", err)
	}
	return s.withHandle(destDirPath, func(h osRootHandle) error {
		// Unpack config upload tarball; if it fails - halt.
		tarReader := tar.NewReader(srcReader)
		checkDeclaredSize := _diskSizeLimiter(cfg)
		checkRealSize := _diskSizeLimiter(cfg)
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
				_, _, err := _createDirForTarItem(h.root, hdr, cfg.dirAccess, checkDeclaredSize, checkRealSize)
				if err != nil {
					return fmt.Errorf("failed to unpack directory '%s': %w", hdr.Name, err)
				}
				continue
			default:
				return fmt.Errorf("failed to unpack file '%s': unsupported file type %d", hdr.Name, hdr.Typeflag)
			}

			if err := h._unpackTarFile(tarReader, hdr, cfg, checkDeclaredSize, checkRealSize); err != nil {
				return err
			}
		}
		if cfg.events.onUnpackComplete != nil {
			if err := cfg.events.onUnpackComplete(); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s osRootHandle) _unpackTarFile(src io.Reader, hdr *tar.Header, cfg tarUnpackConfig, checkDeclaredSize, checkRealSize _diskSizeLimiterFunc,
) error {
	if len(hdr.Name) == 0 {
		return errors.New("failed to unpack file with empty name")
	}
	relFilePath, curDiskSize, err := _createDirForTarItem(s.root, hdr, cfg.dirAccess, checkDeclaredSize, checkRealSize)
	if err != nil {
		return fmt.Errorf("failed to unpack file '%s': %w", hdr.Name, err)
	}
	file, err := s.root.OpenFile(relFilePath, os.O_CREATE|os.O_WRONLY, cfg.fileAccess)
	if err != nil {
		return fmt.Errorf("failed to unpack file '%s': %w", hdr.Name, err)
	}
	// Allow to read exactly one byte too much to know if limit was exceeded.
	limit := 1 + min(cfg.maxItemSize, cfg.maxDiskSize-curDiskSize)
	limitReader := &io.LimitedReader{R: src, N: limit}
	err = _copyAndCloseOut(file, limitReader)
	if err == nil {
		_, err = checkRealSize(relFilePath, limit-limitReader.N)
	}
	if err != nil {
		return fmt.Errorf("failed to unpack file '%s': %w", hdr.Name, err)
	}
	return nil
}

func (s tarFsHandle) withHandle(dir string, fn func(osRootHandle) error) error {
	root, err := s.root.OpenRoot(dir)
	if err != nil {
		return err
	}
	h := osRootHandle{root: root}
	defer h.Close()
	return fn(h)
}

func (s tarFsHandle) _copyDir(src, dst string) error {
	return s.withHandle(src, func(source osRootHandle) error {
		if err := s.root.MkdirAll(dst, 0o777); err != nil {
			return err
		}
		return s.withHandle(dst, func(dest osRootHandle) error {
			return fs.WalkDir(source.root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() {
					return dest.root.MkdirAll(name, 0o777)
				}
				if !entry.Type().IsRegular() {
					return fmt.Errorf("cannot copy non-regular file %s", name)
				}
				info, err := entry.Info()
				if err != nil {
					return err
				}
				in, err := source.root.Open(name)
				if err != nil {
					return err
				}
				defer in.Close() //nolint:errcheck
				out, err := dest.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666|info.Mode()&0o111)
				if err != nil {
					return err
				}
				return _copyAndCloseOut(out, in)
			})
		})
	})
}

func _copyAndCloseOut(out io.WriteCloser, in io.Reader) (err error) {
	defer func() {
		// Close error here mean that the write finalization failed (e.g. a failure flushing to disk)
		if err2 := out.Close(); err2 != nil && err == nil {
			err = err2
		}
	}()
	_, err = io.Copy(out, in)
	return
}

type _diskSizeLimiterFunc func(path string, size int64) (int64, error)

func _createDirForTarItem(
	root *os.Root, hdr *tar.Header, dirAccess os.FileMode, checkDeclaredSize, checkRealSize _diskSizeLimiterFunc,
) (relPath string, diskSize int64, err error) {
	if !filepath.IsLocal(hdr.Name) {
		return "", 0, errors.New("directory escape attempt")
	}
	relPath = filepath.Clean(hdr.Name)
	_, err = checkDeclaredSize(relPath, hdr.Size)
	if err == nil {
		diskSize, err = checkRealSize(relPath, 0)
	}
	if err == nil {
		dirPath := relPath
		if hdr.Typeflag != tar.TypeDir {
			dirPath = filepath.Dir(dirPath)
		}
		err = root.MkdirAll(dirPath, dirAccess)
	}
	return
}

func _diskSizeLimiter(cfg tarUnpackConfig) _diskSizeLimiterFunc {
	// In Golang, map[string]struct{} uses zero bytes for each item, while it uses at least 16 bytes for any other type.
	// For a code trying to prevent a decompression bomb this difference is critical for memory usage.
	type void = struct{}
	var totalSize int64
	uniqueDirEntries := make(map[string]void, 1024) // Not full cfg.maxNumItems, which would use 9.5 MB instantly.
	return func(path string, size int64) (int64, error) {
		totalSize += size
		switch {
		case size > cfg.maxItemSize:
			return totalSize, fmt.Errorf("%w: %d", ErrTarUnpackExceedsLimitFileSize, cfg.maxItemSize)
		case totalSize > cfg.maxDiskSize:
			return totalSize, fmt.Errorf("%w: %d", ErrTarUnpackExceedsLimitDiskSize, cfg.maxDiskSize)
		case len(path) > cfg.maxPathLen:
			return totalSize, fmt.Errorf("%w: %d", ErrTarUnpackExceedsLimitPathLength, cfg.maxPathLen)
		}
		base := path
		for {
			if _, ok := uniqueDirEntries[base]; ok {
				break
			}
			uniqueDirEntries[base] = void{}
			totalSize += cfg.blockSize
			switch {
			case totalSize > cfg.maxDiskSize:
				return totalSize, fmt.Errorf("%w: %d", ErrTarUnpackExceedsLimitDiskSize, cfg.maxDiskSize)
			case len(uniqueDirEntries) > cfg.maxNumItems:
				return totalSize, fmt.Errorf("%w: %d", ErrTarUnpackExceedsLimitFileCount, cfg.maxNumItems)
			}
			parent := filepath.Dir(base)
			if len(parent) == 0 || parent == "." || parent == base {
				break
			}
			base = parent
		}
		return totalSize, nil
	}
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
