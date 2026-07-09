// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/foundriesio/update-server/storage/tuf"
	"github.com/secure-systems-lab/go-securesystemslib/cjson"
	"github.com/stretchr/testify/require"
)

// keyID computes the ota-tuf key id (hex sha256 of PKIX DER) for a public key.
func keyID(t *testing.T, pub crypto.PublicKey) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	require.NoError(t, err)
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}

// buildEd25519Import builds a root.json (at the given version) whose root role
// is held by a freshly generated ed25519 offline key, plus that key's
// AtsKey (private) form. It returns the raw root.json bytes, the candidate
// keys to pass to ImportTuf, and the root role's public key.
func buildEd25519Import(t *testing.T, version int) (rootBytes []byte, keys []tuf.AtsKey, oldPub ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	kid := keyID(t, pub)

	pubKey := tuf.AtsKey{KeyType: "ED25519", KeyValue: tuf.AtsKeyVal{Public: hex.EncodeToString(pub)}}
	secKey := tuf.AtsKey{KeyType: "ED25519", KeyValue: tuf.AtsKeyVal{Private: hex.EncodeToString(priv.Seed())}}

	root := buildRoot(t, version, kid, pubKey)
	// Sign the imported root with its own root key so it is self-consistent.
	msg, err := cjson.EncodeCanonical(root.Signed)
	require.NoError(t, err)
	sig := ed25519.Sign(priv, msg)
	root.Signatures = []tuf.Signature{{KeyID: kid, Method: tuf.SigEd25519, Signature: sig}}

	rootBytes, err = json.MarshalIndent(root, "", "  ")
	require.NoError(t, err)

	return rootBytes, []tuf.AtsKey{secKey}, pub
}

// buildRSAImport builds a root.json and candidate keys whose root role is
// held by a freshly generated RSA offline key.
func buildRSAImport(t *testing.T, version int) (rootBytes []byte, keys []tuf.AtsKey, oldPub *rsa.PublicKey) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	kid := keyID(t, &priv.PublicKey)

	pubDer, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	require.NoError(t, err)
	pubPem := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDer}))
	privPem := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}))

	pubKey := tuf.AtsKey{KeyType: "RSA", KeyValue: tuf.AtsKeyVal{Public: pubPem}}
	secKey := tuf.AtsKey{KeyType: "RSA", KeyValue: tuf.AtsKeyVal{Private: privPem}}

	root := buildRoot(t, version, kid, pubKey)
	rootBytes, err = json.MarshalIndent(root, "", "  ")
	require.NoError(t, err)

	return rootBytes, []tuf.AtsKey{secKey}, &priv.PublicKey
}

// buildRoot builds a minimal root metadata whose root role uses the given key
// and with placeholder online keys for the other roles.
func buildRoot(t *testing.T, version int, rootKeyID string, rootKey tuf.AtsKey) tuf.AtsTufRoot {
	t.Helper()
	keys := map[string]tuf.AtsKey{rootKeyID: rootKey}
	roles := map[tuf.RoleName]tuf.RootRole{
		tuf.RoleRoot: {KeyIDs: []string{rootKeyID}, Threshold: 1},
	}
	for _, role := range []tuf.RoleName{tuf.RoleTargets, tuf.RoleSnapshot, tuf.RoleTimestamp} {
		pub, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		kid := keyID(t, pub)
		keys[kid] = tuf.AtsKey{KeyType: "ED25519", KeyValue: tuf.AtsKeyVal{Public: hex.EncodeToString(pub)}}
		roles[role] = tuf.RootRole{KeyIDs: []string{kid}, Threshold: 1}
	}
	return tuf.AtsTufRoot{
		Signed: tuf.RootMeta{
			SignedCommon: tuf.SignedCommon{Type: tuf.RoleRoot.TufType(), Version: version},
			Keys:         keys,
			Roles:        roles,
		},
	}
}

func TestImportTufEd25519(t *testing.T) {
	fs := newTufTestFs(t)
	// All root versions must be present, so import a contiguous 1..3 chain.
	// The highest version (3) is the trust anchor the rotation chains from.
	rootV1, _, _ := buildEd25519Import(t, 1)
	rootV2, _, _ := buildEd25519Import(t, 2)
	rootBytes, keys, oldPub := buildEd25519Import(t, 3)

	require.NoError(t, fs.Tuf.ImportTuf([][]byte{rootV1, rootV2, rootBytes}, keys))
	require.True(t, fs.Tuf.isInitialized())

	// The imported roots (v1-v3) are stored verbatim and a new root (v4) is created.
	for _, v := range []string{"1.root.json", "2.root.json", "3.root.json", "4.root.json"} {
		_, err := os.Stat(filepath.Join(fs.Config.TufDir(), v))
		require.NoError(t, err, v)
	}

	roots, err := fs.Tuf.GetRoots()
	require.NoError(t, err)
	require.Len(t, roots, 4)
	newRoot := roots[3]
	require.Equal(t, 4, newRoot.Signed.Version)
	require.Len(t, newRoot.Signed.Roles, len(tufRoles))

	// New root must have two signatures: the new root key and the old root key.
	require.Len(t, newRoot.Signatures, 2)
	oldKid := keyID(t, oldPub)
	newKid := newRoot.Signed.Roles[tuf.RoleRoot].KeyIDs[0]
	require.NotEqual(t, oldKid, newKid)

	msg, err := cjson.EncodeCanonical(newRoot.Signed)
	require.NoError(t, err)

	sigs := map[string]tuf.Signature{}
	for _, s := range newRoot.Signatures {
		sigs[s.KeyID] = s
	}
	// Old root key signature verifies over the new canonical payload.
	require.Contains(t, sigs, oldKid)
	require.True(t, ed25519.Verify(oldPub, msg, sigs[oldKid].Signature))

	// New root key signature verifies too.
	require.Contains(t, sigs, newKid)
	newPub, err := hex.DecodeString(newRoot.Signed.Keys[newKid].KeyValue.Public)
	require.NoError(t, err)
	require.True(t, ed25519.Verify(ed25519.PublicKey(newPub), msg, sigs[newKid].Signature))

	// The old root key is not retained in the new root's key set.
	require.NotContains(t, newRoot.Signed.Keys, oldKid)

	// Server keys load and match the new root.
	require.NoError(t, fs.Tuf.LoadTuf())
	require.Equal(t, newKid, fs.Tuf.signers[tuf.RoleRoot].Id)
}

func TestImportTufRSA(t *testing.T) {
	fs := newTufTestFs(t)
	rootBytes, keys, oldPub := buildRSAImport(t, 1)

	require.NoError(t, fs.Tuf.ImportTuf([][]byte{rootBytes}, keys))

	roots, err := fs.Tuf.GetRoots()
	require.NoError(t, err)
	require.Len(t, roots, 2)
	newRoot := roots[1]
	require.Equal(t, 2, newRoot.Signed.Version)
	require.Len(t, newRoot.Signatures, 2)

	oldKid := keyID(t, oldPub)
	msg, err := cjson.EncodeCanonical(newRoot.Signed)
	require.NoError(t, err)
	hashed := sha256.Sum256(msg)

	var oldSig *tuf.Signature
	for i := range newRoot.Signatures {
		if newRoot.Signatures[i].KeyID == oldKid {
			oldSig = &newRoot.Signatures[i]
		}
	}
	require.NotNil(t, oldSig)
	require.Equal(t, tuf.SigRsaPssSha256, oldSig.Method)
	require.NoError(t, rsa.VerifyPSS(oldPub, crypto.SHA256, hashed[:], oldSig.Signature,
		&rsa.PSSOptions{SaltLength: 32, Hash: crypto.SHA256}))
}

func TestImportTufFailsWhenInitialized(t *testing.T) {
	fs := newTufTestFs(t)
	require.NoError(t, fs.Tuf.InitTuf())
	rootBytes, keys, _ := buildEd25519Import(t, 1)
	require.ErrorIs(t, fs.Tuf.ImportTuf([][]byte{rootBytes}, keys), ErrTufAlreadyInitialized)
}

func TestImportTufMissingRootKey(t *testing.T) {
	fs := newTufTestFs(t)

	// Build a root but only supply the public key (no private key material).
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	kid := keyID(t, pub)
	pubKey := tuf.AtsKey{KeyType: "ED25519", KeyValue: tuf.AtsKeyVal{Public: hex.EncodeToString(pub)}}
	root := buildRoot(t, 1, kid, pubKey)
	rootBytes, err := json.MarshalIndent(root, "", "  ")
	require.NoError(t, err)

	err = fs.Tuf.ImportTuf([][]byte{rootBytes}, []tuf.AtsKey{pubKey})
	require.ErrorContains(t, err, "unable to find root key signer for key IDs")
	require.False(t, fs.Tuf.isInitialized())
}

func TestImportTufStoresAllVersions(t *testing.T) {
	fs := newTufTestFs(t)

	// An earlier version (1) is provided too and must be stored verbatim. The
	// latest version (2) is built last so its keys are the ones used to sign
	// the rotation.
	rootV1, _, _ := buildEd25519Import(t, 1)
	rootV2, keys, _ := buildEd25519Import(t, 2)

	require.NoError(t, fs.Tuf.ImportTuf([][]byte{rootV2, rootV1}, keys))

	for _, v := range []string{"1.root.json", "2.root.json", "3.root.json"} {
		_, err := os.Stat(filepath.Join(fs.Config.TufDir(), v))
		require.NoError(t, err, v)
	}

	// The stored v1/v2 files match the provided bytes verbatim.
	storedV1, err := os.ReadFile(filepath.Join(fs.Config.TufDir(), "1.root.json"))
	require.NoError(t, err)
	require.Equal(t, rootV1, storedV1)
}
