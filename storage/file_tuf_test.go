// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/foundriesio/update-server/storage/tuf"
	"github.com/secure-systems-lab/go-securesystemslib/cjson"
	"github.com/stretchr/testify/require"
)

// newTufTestFs returns an initialized filesystem with an HMAC secret set up,
// ready for TUF operations.
func newTufTestFs(t *testing.T) *FsHandle {
	t.Helper()
	fs, err := NewFs(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, fs.Auth.InitHmacSecret())
	return fs
}

func TestInitTuf(t *testing.T) {
	fs := newTufTestFs(t)
	require.False(t, fs.Tuf.isInitialized())

	require.NoError(t, fs.Tuf.InitTuf())
	require.True(t, fs.Tuf.isInitialized())

	// All four role key files exist with secure permissions.
	for _, role := range tufRoles {
		info, err := os.Stat(fs.Tuf.keyPath(role))
		require.NoError(t, err, "key for role %s", role)
		require.Equal(t, secureFileAccess, info.Mode().Perm())
	}

	// An initial v1 root metadata file exists.
	_, err := os.Stat(filepath.Join(fs.Config.TufDir(), "1.root.json"))
	require.NoError(t, err)
}

func TestInitTufFailsWhenAlreadyInitialized(t *testing.T) {
	fs := newTufTestFs(t)
	require.NoError(t, fs.Tuf.InitTuf())
	require.ErrorIs(t, fs.Tuf.InitTuf(), ErrTufAlreadyInitialized)
}

func TestInitTufRequiresHmacSecret(t *testing.T) {
	fs, err := NewFs(t.TempDir())
	require.NoError(t, err)
	require.Error(t, fs.Tuf.InitTuf())
	require.False(t, fs.Tuf.isInitialized())
}

func TestKeysAreEncryptedOnDisk(t *testing.T) {
	fs := newTufTestFs(t)
	require.NoError(t, fs.Tuf.InitTuf())

	roots, err := fs.Tuf.GetRoots()
	require.NoError(t, err)
	require.Len(t, roots, 1)

	// The raw key file must not contain any of the public key hex (which would
	// indicate the AtsKey JSON was stored unencrypted).
	raw, err := os.ReadFile(fs.Tuf.keyPath(tuf.RoleRoot))
	require.NoError(t, err)
	for _, key := range roots[0].Signed.Keys {
		require.NotContains(t, string(raw), key.KeyValue.Public)
	}
	require.NotContains(t, string(raw), "keytype")
}

func TestLoadTufNotInitialized(t *testing.T) {
	fs := newTufTestFs(t)
	require.ErrorIs(t, fs.Tuf.LoadTuf(), ErrTufNotInitialized)
}

func TestLoadTuf(t *testing.T) {
	fs := newTufTestFs(t)
	require.NoError(t, fs.Tuf.InitTuf())
	require.NoError(t, fs.Tuf.LoadTuf())

	require.Len(t, fs.Tuf.signers, len(tufRoles))

	roots, err := fs.Tuf.GetRoots()
	require.NoError(t, err)
	root := roots[0]

	// Each loaded signer must match the key id recorded in root metadata.
	for _, role := range tufRoles {
		signer := fs.Tuf.signers[role]
		require.NotNil(t, signer, "role %s", role)
		rr := root.Signed.Roles[role]
		require.NotNil(t, rr)
		require.Equal(t, []string{signer.Id}, rr.KeyIDs)
		require.Equal(t, 1, rr.Threshold)
	}
}

func TestGetRoots(t *testing.T) {
	fs := newTufTestFs(t)
	before := time.Now().UTC()
	require.NoError(t, fs.Tuf.InitTuf())

	roots, err := fs.Tuf.GetRoots()
	require.NoError(t, err)
	require.Len(t, roots, 1)

	root := roots[0]
	require.Equal(t, "Root", root.Signed.Type)
	require.Equal(t, 1, root.Signed.Version)
	require.False(t, root.Signed.ConsistentSnapshot)
	require.Len(t, root.Signed.Keys, len(tufRoles))
	require.Len(t, root.Signed.Roles, len(tufRoles))

	// Root should expire roughly 20 years out (matching RootExpiration).
	expectedExpiry := before.Add(fs.Tuf.RootExpiration)
	require.WithinDuration(t, expectedExpiry, root.Signed.Expires, time.Minute)

	// Exactly one signature, by the root key, and it must verify.
	require.Len(t, root.Signatures, 1)
	sig := root.Signatures[0]
	require.Equal(t, tuf.SigEd25519, sig.Method)

	pubHex := root.Signed.Keys[sig.KeyID].KeyValue.Public
	require.NotEmpty(t, pubHex)
	pub, err := hex.DecodeString(pubHex)
	require.NoError(t, err)

	msg, err := cjson.EncodeCanonical(root.Signed)
	require.NoError(t, err)
	require.True(t, ed25519.Verify(ed25519.PublicKey(pub), msg, sig.Signature),
		"root signature must verify against canonical signed payload")
}

func TestGetRootsEmpty(t *testing.T) {
	fs := newTufTestFs(t)
	roots, err := fs.Tuf.GetRoots()
	require.NoError(t, err)
	require.Empty(t, roots)
}

func TestRootMetaJSONFormat(t *testing.T) {
	fs := newTufTestFs(t)
	require.NoError(t, fs.Tuf.InitTuf())

	content, err := os.ReadFile(filepath.Join(fs.Config.TufDir(), "1.root.json"))
	require.NoError(t, err)

	// Sanity check the on-disk shape matches the foundries/ota-tuf format.
	var generic struct {
		Signatures []json.RawMessage `json:"signatures"`
		Signed     struct {
			Type  string `json:"_type"`
			Roles map[string]struct {
				Threshold int `json:"threshold"`
			} `json:"roles"`
		} `json:"signed"`
	}
	require.NoError(t, json.Unmarshal(content, &generic))
	require.Equal(t, "Root", generic.Signed.Type)
	for _, role := range []string{"root", "targets", "snapshot", "timestamp"} {
		_, ok := generic.Signed.Roles[role]
		require.True(t, ok, "missing role %s", role)
	}
	require.True(t, strings.Contains(string(content), "\"keytype\": \"ED25519\""))
}
