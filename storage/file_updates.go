// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	// The biggest docker layer we've seen so far is a bit above 1 GB.
	// Regular customers' build is usually a few GB.
	maxUpdateTarballItemFileSize  int64 = 1 << 31 // 2 GB
	maxUpdateTarballTotalDiskSize int64 = 1 << 35 // 32 GB
)

var ErrInvalidUpdate = errors.New("invalid update archive")

type Update struct {
	Tag        string `json:"tag"`
	Name       string `json:"name"`
	UploadedAt int64  `json:"uploaded-at"`
	UploadedBy string `json:"uploaded-by"`

	DeviceCount int `json:"device-count"`
}

type updatesFsHandleWrap struct {
	osRootHandle
	Apps     UpdatesFsHandle
	Ostree   UpdatesFsHandle
	Tuf      UpdatesFsHandle
	Rollouts RolloutsFsHandle
	Logs     UpdatesFsHandle
}

func (s *updatesFsHandleWrap) init() {
	root := s.root
	s.Apps.root = root
	s.Apps.category = UpdatesAppsDir
	s.Ostree.root = root
	s.Ostree.category = UpdatesOstreeDir
	s.Rollouts.root = root
	s.Rollouts.category = UpdatesRolloutsDir
	s.Tuf.root = root
	s.Tuf.category = UpdatesTufDir
	s.Logs.root = root
	s.Logs.category = UpdatesLogsDir
}

// Delete removes the entire on-disk directory for an update (all categories:
// tuf, ostree_repo, apps, rollouts, logs). A missing directory is not an error.
func (s updatesFsHandleWrap) Delete(update string) error {
	if err := s.root.RemoveAll(update); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("error deleting file storage for update %s: %w", update, err)
	}
	return nil
}

// checkUpdateTargets ensures that the update contains a valid targets.json file by looking for:
//   - is it valid JSON?
//   - does it have a target with the given tag
func checkUpdateTargets(targetsPath, tag string) error {
	content, err := os.ReadFile(targetsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("missing required targets.json file in tuf directory")
		}
		return fmt.Errorf("error reading targets.json: %w", err)
	}

	var targets struct {
		Signed struct {
			Targets map[string]struct {
				Custom struct {
					Tags []string `json:"tags"`
				} `json:"custom"`
			} `json:"targets"`
		} `json:"signed"`
	}

	if err = json.Unmarshal(content, &targets); err != nil {
		return fmt.Errorf("error parsing targets.json: %w", err)
	}

	for _, t := range targets.Signed.Targets {
		if slices.Contains(t.Custom.Tags, tag) {
			return nil
		}
	}
	return fmt.Errorf("no target with tag '%s' found in targets.json", tag)
}

func (s updatesFsHandleWrap) SaveUpload(tag, update string, payload io.Reader, tufCreateFunc func(string) error, onCleanupFailure func(error)) error {
	const (
		appsDir   = UpdatesAppsDir + string(filepath.Separator)
		ostreeDir = UpdatesOstreeDir + string(filepath.Separator)
		tufDir    = UpdatesTufDir + string(filepath.Separator)
	)
	var sawTuf, sawOstree, sawApps bool
	txDir := ".update-upload-" + rand.Text()[:10]
	root, destDir := filepath.Split(s.root.Name())
	destDir = filepath.Join(destDir, update)
	tarRoot, err := os.OpenRoot(root)
	if err != nil {
		return fmt.Errorf("unable to open upload storage: %w", err)
	}
	h := tarFsHandle{osRootHandle{root: tarRoot}}
	defer h.Close()
	return h.unpackTar(payload, destDir,
		TarUnpackReplaceDest(true), // Replace updates with the same name - uniqueness is checked on the database level.
		TarUnpackUseTmpFile("update.tar"),
		TarUnpackUseTmpDir(txDir),
		TarUnpackMaxItemFileSize(maxUpdateTarballItemFileSize),
		TarUnpackMaxTotalDiskSize(maxUpdateTarballTotalDiskSize),
		TarUnpackOnEvents(tarUnpackEvents{
			onTmpCleanupError: onCleanupFailure,
			onTarHeaderSeen: func(hdr *TarHeader) (skip bool, err error) {
				// If any check below fails due to header name being unclean - whose problem is that?
				// For tarballs with many files, below boolean algebra is way faster than a switch over name patterns.
				sawApps = sawApps || strings.HasPrefix(hdr.Name, appsDir)
				sawOstree = sawOstree || strings.HasPrefix(hdr.Name, ostreeDir)
				sawTuf = sawTuf || strings.HasPrefix(hdr.Name, tufDir)
				return
			},
			onUnpackComplete: func() error {
				if !sawOstree && !sawApps {
					return fmt.Errorf("%w: must contain %q and/or %q directory",
						ErrInvalidUpdate, UpdatesOstreeDir, UpdatesAppsDir)
				}
				if !sawTuf {
					// When TUF is enabled the server generates the metadata from
					// the uploaded ostree/apps content, so the tuf directory is
					// optional. Otherwise it is required.
					if tufCreateFunc == nil {
						return fmt.Errorf("%w: missing required %q directory", ErrInvalidUpdate, UpdatesTufDir)
					}
					if err := tufCreateFunc(filepath.Join(root, txDir, "unpacked")); err != nil {
						return fmt.Errorf("%w: unable to generate TUF metadata: %v", ErrInvalidUpdate, err) //nolint:errorlint
					}
				}

				path := filepath.Join(root, txDir, "unpacked/tuf/targets.json")
				if err := checkUpdateTargets(path, tag); err != nil {
					return fmt.Errorf("%w: %v", ErrInvalidUpdate, err) //nolint:errorlint
				}
				return nil
			},
		}),
	)
}

type UpdatesFsHandle struct {
	osRootHandle
	category string
}

func (s UpdatesFsHandle) FilePath(update, name string) string {
	return filepath.Join(s.root.Name(), update, s.category, name)
}

func (s UpdatesFsHandle) ReadFile(update, name string) (string, error) {
	var content string
	err := s.withHandle(update, false, func(h osRootHandle) error {
		var err error
		content, err = h.readFile(name, false)
		return err
	})
	if err != nil {
		err = fmt.Errorf("error reading %s file for update %s: %w", s.category, update, err)
	}
	return content, err
}

func (s UpdatesFsHandle) LatestRootMetaName(update string) (string, error) {
	files, err := s.listFiles(update, false)
	if err != nil {
		return "", fmt.Errorf("error find latest root metadata: %w", err)
	} else if len(files) == 0 {
		return "", fmt.Errorf("no metadata files found for update %s", update)
	}
	slices.SortFunc(files, func(a, b string) int {
		aIsRoot := strings.HasSuffix(a, ".root.json")
		bIsRoot := strings.HasSuffix(b, ".root.json")
		if aIsRoot && bIsRoot {
			// Convert both into 0 padded strings to ensure proper lexicographical comparison
			// 13 = makes 1.root.json become 001.root.json to support 999 versions of root.json
			a = fmt.Sprintf("%013s", a)
			b = fmt.Sprintf("%013s", b)
			return strings.Compare(b, a)
		} else if aIsRoot {
			return -1
		} else if bIsRoot {
			return 1
		}
		return 1
	})
	return files[0], nil
}

func (s UpdatesFsHandle) TailFileLines(update, name string, stop DoneChan) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		err := s.withHandle(update, false, func(h osRootHandle) error {
			h.readFileLines(name, false, stop)(yield)
			return nil
		})
		if err != nil {
			yield("", err)
		}
	}
}

func (s UpdatesFsHandle) WriteFile(update, name, content string) error {
	return s.withHandle(update, true, func(h osRootHandle) error {
		if err := h.writeFile(name, content, defaultFileAccess); err != nil {
			return fmt.Errorf("error writing %s file for update %s: %w", s.category, update, err)
		}
		return nil
	})
}

func (s UpdatesFsHandle) AppendFile(update, name, content string) error {
	return s.withHandle(update, true, func(h osRootHandle) error {
		if err := h.appendFile(name, content, defaultFileAccess); err != nil {
			return fmt.Errorf("error appending %s file for update %s: %w", s.category, update, err)
		}
		return nil
	})
}

func (s UpdatesFsHandle) withHandle(update string, forUpdate bool, fn func(osRootHandle) error) error {
	dir := filepath.Join(update, s.category)
	if forUpdate {
		if err := s.root.MkdirAll(dir, defaultDirAccess); err != nil {
			return fmt.Errorf("unable to create %s file storage for update %s: %w", s.category, update, err)
		}
	}
	root, err := s.root.OpenRoot(dir)
	if err != nil {
		return err
	}
	h := osRootHandle{root: root}
	defer h.Close()
	return fn(h)
}

func (s UpdatesFsHandle) listFiles(update string, sortByModTime bool) ([]string, error) {
	var files []string
	err := s.withHandle(update, false, func(h osRootHandle) error {
		var err error
		files, err = h.matchFiles("", sortByModTime)
		return err
	})
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	return files, err
}

type RolloutsFsHandle struct {
	UpdatesFsHandle
}

func (s RolloutsFsHandle) ListFiles(update string) ([]string, error) {
	return s.listFiles(update, true)
}

func (s RolloutsFsHandle) AppendJournal(content string) error {
	return s.appendFile(rolloutJournalFile+partialFileSuffix, content, defaultFileAccess)
}

func (s RolloutsFsHandle) RolloverJournal() (err error) {
	if err = s.root.Rename(rolloutJournalFile+partialFileSuffix, rolloutJournalFile); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// No new writes into a journal since the last rollover - that's just fine.
			err = nil
		}
	}
	return
}

func (s RolloutsFsHandle) ReadJournal() iter.Seq2[string, error] {
	return s.readFileLines(rolloutJournalFile, true, nil)
}
