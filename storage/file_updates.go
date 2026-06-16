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

var ErrInvalidUpdate = errors.New("invalid update archive")

type Update struct {
	Name       string `json:"name"`
	UploadedAt int64  `json:"uploaded-at"`
	UploadedBy string `json:"uploaded-by"`
}

type updatesFsHandleWrap struct {
	baseFsHandle
	Apps     UpdatesFsHandle
	Ostree   UpdatesFsHandle
	Tuf      UpdatesFsHandle
	Rollouts RolloutsFsHandle
	Logs     UpdatesFsHandle
}

func (s *updatesFsHandleWrap) init(root string) {
	s.root = root
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

func (s updatesFsHandleWrap) SaveUpload(tag, update string, payload io.Reader, onCleanupFailure func(error), onCommit func() error) error {
	const (
		appsDir   = UpdatesAppsDir + string(filepath.Separator)
		ostreeDir = UpdatesOstreeDir + string(filepath.Separator)
		tufDir    = UpdatesTufDir + string(filepath.Separator)
	)
	var sawTuf, sawOstree, sawApps bool
	txDir := ".update-upload-" + rand.Text()[:10]
	root, destDir := filepath.Split(s.root)
	destDir = filepath.Join(destDir, tag, update)
	h := tarFsHandle{root: root}
	return h.unpackTar(payload, destDir,
		TarUnpackReplaceDest(false), // Fail if the update with the same tag and name already exists.
		TarUnpackUseTmpFile("update.tar"),
		TarUnpackUseTmpDir(txDir),
		TarUnpackOnEvents(tarUnpackEvents{
			onTmpCleanupError: onCleanupFailure,
			onCommit:          onCommit,
			onTarHeaderSeen: func(hdr *TarHeader) (skip bool, err error) {
				// If any check below fails due to header name being unclean - whose problem is that?
				// For tarballs with many files, below boolean algebra is way faster than a switch over name patterns.
				sawApps = sawApps || strings.HasPrefix(hdr.Name, appsDir)
				sawOstree = sawOstree || strings.HasPrefix(hdr.Name, ostreeDir)
				sawTuf = sawTuf || strings.HasPrefix(hdr.Name, tufDir)
				return
			},
			onUnpackComplete: func() error {
				if !sawTuf {
					return fmt.Errorf("%w: missing required %q directory", ErrInvalidUpdate, UpdatesTufDir)
				}
				if !sawOstree && !sawApps {
					return fmt.Errorf("%w: must contain %q and/or %q directory",
						ErrInvalidUpdate, UpdatesOstreeDir, UpdatesAppsDir)
				}

				path := filepath.Join(root, txDir, "unpacked/tuf/targets.json")
				if err := checkUpdateTargets(path, tag); err != nil {
					return fmt.Errorf("%w: %v", ErrInvalidUpdate, err)
				}
				return nil
			},
		}),
	)
}

type UpdatesFsHandle struct {
	baseFsHandle
	category string
}

func (s UpdatesFsHandle) FilePath(tag, update, name string) string {
	return filepath.Join(s.root, tag, update, s.category, name)
}

func (s UpdatesFsHandle) ReadFile(tag, update, name string) (string, error) {
	h, _ := s.updateLocalHandle(tag, update, false)
	content, err := h.readFile(name, false)
	if err != nil {
		err = fmt.Errorf("error reading %s file for tag %s update %s: %w", s.category, tag, update, err)
	}
	return content, err
}

func (s UpdatesFsHandle) LatestRootMetaName(tag, update string) (string, error) {
	h, _ := s.updateLocalHandle(tag, update, false)
	files, err := h.matchFiles("", false)
	if err != nil {
		return "", fmt.Errorf("error find latest root metadata: %w", err)
	} else if len(files) == 0 {
		return "", fmt.Errorf("no metadata files found for tag %s update %s", tag, update)
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

func (s UpdatesFsHandle) TailFileLines(tag, update, name string, stop DoneChan) iter.Seq2[string, error] {
	h, _ := s.updateLocalHandle(tag, update, false)
	return h.readFileLines(name, false, stop)
}

func (s UpdatesFsHandle) WriteFile(tag, update, name, content string) error {
	if h, err := s.updateLocalHandle(tag, update, true); err != nil {
		return err
	} else if err = h.writeFile(name, content, defaultFileAccess); err != nil {
		return fmt.Errorf("error writing %s file for tag %s update %s: %w", s.category, tag, update, err)
	}
	return nil
}

func (s UpdatesFsHandle) AppendFile(tag, update, name, content string) error {
	if h, err := s.updateLocalHandle(tag, update, true); err != nil {
		return err
	} else if err = h.appendFile(name, content, defaultFileAccess); err != nil {
		return fmt.Errorf("error appending %s file for tag %s update %s: %w", s.category, tag, update, err)
	}
	return nil
}

func (s UpdatesFsHandle) updateLocalHandle(tag, update string, forUpdate bool) (h baseFsHandle, err error) {
	h.root = filepath.Join(s.root, tag, update, s.category)
	if forUpdate {
		if err = h.mkdirs(defaultDirAccess, true); err != nil {
			err = fmt.Errorf("unable to create %s file storage for tag %s update %s: %w", s.category, tag, update, err)
		}
	}
	return
}

type RolloutsFsHandle struct {
	UpdatesFsHandle
}

func (s RolloutsFsHandle) ListFiles(tag, update string) ([]string, error) {
	h, _ := s.updateLocalHandle(tag, update, false)
	return h.matchFiles("", true)
}

func (s RolloutsFsHandle) AppendJournal(content string) error {
	return s.appendFile(rolloutJournalFile+partialFileSuffix, content, defaultFileAccess)
}

func (s RolloutsFsHandle) RolloverJournal() (err error) {
	from := filepath.Join(s.root, rolloutJournalFile+partialFileSuffix)
	to := filepath.Join(s.root, rolloutJournalFile)
	if err = os.Rename(from, to); err != nil {
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
