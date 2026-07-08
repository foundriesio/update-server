// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package ostree

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Repo struct {
	path string
}

func NewRepo(path string) *Repo {
	return &Repo{path: path}
}

func (r *Repo) objectPath(hash, ext string) string {
	return filepath.Join(r.path, "objects", hash[:2], hash[2:]+ext)
}

// isValidObjectHash reports whether hash is a 64-character lowercase hex string,
// the form of an OSTree object checksum. Validating before constructing object
// paths guards against panics and path traversal from malformed upload content.
func isValidObjectHash(hash string) bool {
	if len(hash) != 64 {
		return false
	}
	for i := 0; i < len(hash); i++ {
		c := hash[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func (r *Repo) ReadRef(ref string) (string, error) {
	for _, base := range []string{"heads", "remotes"} {
		data, err := os.ReadFile(filepath.Join(r.path, "refs", base, ref))
		if err == nil {
			hash := strings.TrimSpace(string(data))
			if !isValidObjectHash(hash) {
				return "", fmt.Errorf("ref %q does not contain a valid object hash", ref)
			}
			return hash, nil
		}
	}
	return "", fmt.Errorf("ref not found: %s", ref)
}

// gvariantOffsetSize returns the byte width of GVariant framing offsets for a
// container of the given total byte length.
func gvariantOffsetSize(n int) int {
	switch {
	case n <= 0xff:
		return 1
	case n <= 0xffff:
		return 2
	default:
		return 4
	}
}

func readLE(data []byte, pos, size int) int {
	switch size {
	case 1:
		return int(data[pos])
	case 2:
		return int(binary.LittleEndian.Uint16(data[pos:]))
	default:
		return int(binary.LittleEndian.Uint32(data[pos:]))
	}
}

// rootDirtreeHash extracts the root dirtree checksum from a raw commit object.
//
// OSTree commit GVariant type: (a{sv}aya(say)sstayay)
// Fields: a{sv} ay a(say) s s t ay ay
// Variable-length fields needing framing (all except the last ay): 6 fields.
// Framing offsets are stored at the end in reverse field order.
// After the body string (field 5), alignment to 8 for the uint64 timestamp (t),
// then 32-byte dirtree hash, then 32-byte dirmeta hash, then 6 framing bytes.
//
// Framing layout (last 6 bytes, each 1 byte since total commit < 256):
//
//	[0]: end of ay(dirtree)   [1]: end of s(body)   [2]: end of s(subject)
//	[3]: end of a(say)        [4]: end of ay(parent) [5]: end of a{sv}
func rootDirtreeHash(data []byte) (string, error) {
	n := len(data)
	offSize := gvariantOffsetSize(n)
	// 6 framing offsets at the tail
	if n < 6*offSize+8+32 {
		return "", fmt.Errorf("commit object too small (%d bytes)", n)
	}
	framingBase := n - 6*offSize
	// framing offsets in reverse field order; index 1 = end of body string
	oBody := readLE(data, framingBase+offSize, offSize)
	if oBody >= framingBase {
		return "", fmt.Errorf("commit body offset %d out of range", oBody)
	}
	// timestamp (uint64) follows body string, aligned to 8 bytes
	tsStart := (oBody + 7) &^ 7
	dirtreeStart := tsStart + 8
	if dirtreeStart+32 > framingBase {
		return "", fmt.Errorf("commit dirtree region overruns framing table")
	}
	return hex.EncodeToString(data[dirtreeStart : dirtreeStart+32]), nil
}

// lookupDirtree parses a raw dirtree object (GVariant type (a(say)a(sayay))) and
// returns the file hash if name is a file, or the dirtree hash if name is a subdir.
//
// Array framing: each GVariant array of variable-length elements stores N element-end
// offsets at the end of the array.  The offset size is determined by the array length.
func lookupDirtree(data []byte, name string) (fileHash, subdirHash string, err error) {
	n := len(data)
	if n == 0 {
		return "", "", fmt.Errorf("empty dirtree object")
	}
	// Outer tuple (A B): one framing offset at the end = end of A (files array).
	offSize := gvariantOffsetSize(n)
	if n < offSize {
		return "", "", fmt.Errorf("dirtree too small")
	}
	filesEnd := readLE(data, n-offSize, offSize)
	if filesEnd > n-offSize {
		return "", "", fmt.Errorf("dirtree files-end offset %d out of range", filesEnd)
	}

	if fh := lookupGVArray(data[:filesEnd], name); fh != "" {
		return fh, "", nil
	}
	if dh := lookupGVArray(data[filesEnd:n-offSize], name); dh != "" {
		return "", dh, nil
	}
	return "", "", nil
}

// lookupGVArray searches a GVariant a(say) or a(sayay) array for name and returns
// the first 32-byte hash that follows the name's null terminator.
func lookupGVArray(data []byte, name string) string {
	n := len(data)
	if n == 0 {
		return ""
	}
	offSize := gvariantOffsetSize(n)
	// Last offSize bytes = framing[N-1] = end of last element = start of framing table.
	lastElemEnd := readLE(data, n-offSize, offSize)
	if lastElemEnd > n-offSize {
		return ""
	}
	numElems := (n - lastElemEnd) / offSize
	if numElems == 0 {
		return ""
	}

	prev := 0
	for i := 0; i < numElems; i++ {
		end := readLE(data, lastElemEnd+i*offSize, offSize)
		if end <= prev || end > lastElemEnd {
			return ""
		}
		elem := data[prev:end]
		// Each element is (s ay...). Find the null terminator of s.
		nullPos := bytes.IndexByte(elem, 0)
		if nullPos < 0 {
			return ""
		}
		if string(elem[:nullPos]) == name {
			hashStart := nullPos + 1
			if hashStart+32 > len(elem) {
				return ""
			}
			return hex.EncodeToString(elem[hashStart : hashStart+32])
		}
		prev = end
	}
	return ""
}

// readContentObject returns the raw file content for a content object hash,
// handling both "bare" repositories (uncompressed ".file" objects) and
// "archive"/"archive-z2" repositories (".filez" objects: a big-endian uint32
// header size, 4 bytes of alignment padding, the GVariant file header, followed
// by raw-DEFLATE-compressed content).
func (r *Repo) readContentObject(fileHash string) ([]byte, error) {
	if data, err := os.ReadFile(r.objectPath(fileHash, ".file")); err == nil {
		return data, nil
	}

	raw, err := os.ReadFile(r.objectPath(fileHash, ".filez"))
	if err != nil {
		return nil, fmt.Errorf("reading file object %s: %w", fileHash[:8], err)
	}
	if len(raw) < 8 {
		return nil, fmt.Errorf("filez object %s too small (%d bytes)", fileHash[:8], len(raw))
	}
	// 4-byte big-endian header size, then 4 bytes of alignment padding.
	headerSize := int(binary.BigEndian.Uint32(raw[:4]))
	contentStart := 8 + headerSize
	if headerSize == 0 || contentStart > len(raw) {
		return nil, fmt.Errorf("filez object %s has invalid header size %d", fileHash[:8], headerSize)
	}

	zr := flate.NewReader(bytes.NewReader(raw[contentStart:]))
	defer zr.Close() // nolint:errcheck
	content, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("decompressing file object %s: %w", fileHash[:8], err)
	}
	return content, nil
}

// ReadFile returns the contents of filePath from the given ref in the repo.
// filePath should be an absolute path, e.g. "/usr/lib/sota/conf.d/40-hardware-id.toml".
func (r *Repo) ReadFile(ref, filePath string) ([]byte, error) {
	commitHash, err := r.ReadRef(ref)
	if err != nil {
		return nil, err
	}

	commitData, err := os.ReadFile(r.objectPath(commitHash, ".commit"))
	if err != nil {
		return nil, fmt.Errorf("reading commit object: %w", err)
	}

	dirtreeHash, err := rootDirtreeHash(commitData)
	if err != nil {
		return nil, fmt.Errorf("parsing commit: %w", err)
	}

	parts := strings.Split(strings.Trim(filePath, "/"), "/")
	for _, part := range parts[:len(parts)-1] {
		dirtreeData, err := os.ReadFile(r.objectPath(dirtreeHash, ".dirtree"))
		if err != nil {
			return nil, fmt.Errorf("reading dirtree %s: %w", dirtreeHash[:8], err)
		}
		_, dirtreeHash, err = lookupDirtree(dirtreeData, part)
		if err != nil {
			return nil, fmt.Errorf("parsing dirtree for %q: %w", part, err)
		}
		if dirtreeHash == "" {
			return nil, fmt.Errorf("directory %q not found", part)
		}
	}

	filename := parts[len(parts)-1]
	dirtreeData, err := os.ReadFile(r.objectPath(dirtreeHash, ".dirtree"))
	if err != nil {
		return nil, fmt.Errorf("reading dirtree %s: %w", dirtreeHash[:8], err)
	}
	fileHash, _, err := lookupDirtree(dirtreeData, filename)
	if err != nil {
		return nil, fmt.Errorf("parsing dirtree for %q: %w", filename, err)
	}
	if fileHash == "" {
		return nil, fmt.Errorf("file %q not found in %s", filename, filepath.Dir(filePath))
	}

	return r.readContentObject(fileHash)
}

// ListHeads returns the names of all refs under refs/heads, including refs nested
// in subdirectories (reported with '/' separators, e.g. "foo/bar").
func (r *Repo) ListHeads() ([]string, error) {
	headsDir := filepath.Join(r.path, "refs", "heads")
	var refs []string
	err := filepath.WalkDir(headsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(headsDir, path)
		if err != nil {
			return err
		}
		refs = append(refs, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing refs/heads: %w", err)
	}
	return refs, nil
}
