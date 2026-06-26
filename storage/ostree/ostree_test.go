// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package ostree_test

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/foundriesio/update-server/storage/ostree"
	"github.com/stretchr/testify/require"
)

// These tests require a real OSTree repo at /sysroot/ostree/repo and will be
// skipped if it is not present.

func TestReadFile(t *testing.T) {
	const repoPath = "/sysroot/ostree/repo"
	const ref = "intel-corei7-64-lmp"
	const filePath = "/usr/lib/sota/conf.d/40-hardware-id.toml"
	const expected = "[provision]\nprimary_ecu_hardware_id = \"intel-corei7-64\"\n"

	repo := ostree.NewRepo(repoPath)

	_, err := repo.ReadRef(ref)
	if err != nil {
		t.Skipf("OSTree repo not available (%v)", err)
	}

	content, err := repo.ReadFile(ref, filePath)
	require.NoError(t, err)
	require.Equal(t, expected, string(content))
}

func TestReadFileNotFound(t *testing.T) {
	const repoPath = "/sysroot/ostree/repo"
	const ref = "intel-corei7-64-lmp"

	repo := ostree.NewRepo(repoPath)

	_, err := repo.ReadRef(ref)
	if err != nil {
		t.Skipf("OSTree repo not available (%v)", err)
	}

	_, err = repo.ReadFile(ref, "/usr/lib/sota/conf.d/nonexistent.toml")
	require.ErrorContains(t, err, "not found")
}

// TestParseEmbedded exercises commit and dirtree parsing with real bytes captured
// from an OSTree repo, without needing the repo present at test time.
func TestParseEmbedded(t *testing.T) {
	// Real commit object from intel-corei7-64-lmp ref.
	commitB64 := "b3N0cmVlLnJlZi1iaW5kaW5nAAAAAAAAaW50ZWwtY29yZWk3LTY0LWxtcAAUAGFzEzG1AK7bkK3BehHydWbJRP6wPpY4vI+hYupk5Q+pVKRATCJhbmR5LXRlc3Qtd2l0aC1zY3JpcHQiAAAAAAAAAAAAAABn+ChCa+Nk2LmOpzozokSSqmXmf2OtHDlC+3CH7tP9Ui8XXvtEag7xG3zBZ/O2A+WFx+7utnX6pBLV7HP2KYjrC2xUiJhralJSMg=="
	// Real dirtree object for /usr/lib/sota/conf.d in that commit.
	confdDirtreeB64 := "NDAtaGFyZHdhcmUtaWQudG9tbAC1ZC8Sl/gPnTzhmR+/MldHJ2cJ86zkuVmJlEO4r3IvSBQ0Ni1wa2NzMTEtbGFiZWwudG9tbADMqpJ1rI4WLNXdRi4t7EcPxlwFlOexZusMtkDgXeAKuBU1a20="
	// Raw content of the target file.
	fileContent := "[provision]\nprimary_ecu_hardware_id = \"intel-corei7-64\"\n"

	commitBytes, err := base64.StdEncoding.DecodeString(commitB64)
	require.NoError(t, err)
	confdDirtreeBytes, err := base64.StdEncoding.DecodeString(confdDirtreeB64)
	require.NoError(t, err)

	const commitHash = "f053412485a867bf2853b447367330940fd46e0527edef77ce9e7cedf8a043fd"
	const rootDirtreeHash = "6be364d8b98ea73a33a24492aa65e67f63ad1c3942fb7087eed3fd522f175efb"
	const confdDirtreeHash = "e0a8be1e0b2efdd2c63a388bc7d212433673081fe8823f74dd7ef7153ddb246e"
	const fileHash = "b5642f1297f80f9d3ce1991fbf325747276709f3ace4b959899443b8af722f48"

	// Build a fake repo tree containing just the objects we need.
	repoPath := t.TempDir()
	writeObj := func(hash, ext string, data []byte) {
		dir := filepath.Join(repoPath, "objects", hash[:2])
		require.NoError(t, os.MkdirAll(dir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, hash[2:]+ext), data, 0644))
	}

	// We don't walk the full dirtree chain here — only seed the objects actually
	// exercised by ReadFile for the conf.d path segment.
	writeObj(commitHash, ".commit", commitBytes)
	writeObj(confdDirtreeHash, ".dirtree", confdDirtreeBytes)
	writeObj(fileHash, ".file", []byte(fileContent))

	// Stub out intermediate dirtrees by making them point directly to confdDirtreeHash.
	// Each is a minimal dirtree containing only one subdir entry for the next path
	// component, so we create them synthetically.
	writeMinimalDirtree := func(hash, childName, childDirtreeHash string) {
		data := buildMinimalSubdirDirtree(childName, childDirtreeHash)
		writeObj(hash, ".dirtree", data)
	}

	// Chain: rootDirtreeHash/usr -> usrDT/lib -> libDT/sota -> sotaDT/conf.d -> confdDirtreeHash
	const usrDT = "1100000000000000000000000000000000000000000000000000000000000001"
	const libDT = "1100000000000000000000000000000000000000000000000000000000000002"
	const sotaDT = "1100000000000000000000000000000000000000000000000000000000000003"

	writeMinimalDirtree(rootDirtreeHash, "usr", usrDT)
	writeMinimalDirtree(usrDT, "lib", libDT)
	writeMinimalDirtree(libDT, "sota", sotaDT)
	writeMinimalDirtree(sotaDT, "conf.d", confdDirtreeHash)

	require.NoError(t, os.MkdirAll(filepath.Join(repoPath, "refs", "heads"), 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(repoPath, "refs", "heads", "test-ref"),
		[]byte(commitHash+"\n"), 0644,
	))

	repo := ostree.NewRepo(repoPath)
	content, err := repo.ReadFile("test-ref", "/usr/lib/sota/conf.d/40-hardware-id.toml")
	require.NoError(t, err)
	require.Equal(t, fileContent, string(content))
}

// buildMinimalSubdirDirtree constructs a GVariant (a(say)a(sayay)) dirtree with
// zero files and one subdirectory entry, suitable for use in unit tests.
func buildMinimalSubdirDirtree(name, dirtreeHash string) []byte {
	hashBytes := hexDecode(dirtreeHash)
	dirmeta := make([]byte, 32) // zero dirmeta hash

	// Subdir element: name\0 + 32-byte dirtree + 32-byte dirmeta
	elem := []byte(name + "\x00")
	elem = append(elem, hashBytes...)
	elem = append(elem, dirmeta...)

	// Dirs array: one element + 1-byte framing offset (end of element)
	dirsArray := append(elem, byte(len(elem)))

	// Outer tuple: files array is empty (0 bytes), dirs array follows.
	// One outer framing byte = 0 (end of empty files array).
	result := append(dirsArray, 0x00)
	return result
}

func hexDecode(s string) []byte {
	b := make([]byte, len(s)/2)
	for i := range b {
		hi := hexNibble(s[i*2])
		lo := hexNibble(s[i*2+1])
		b[i] = (hi << 4) | lo
	}
	return b
}

func hexNibble(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	default:
		return 0
	}
}
