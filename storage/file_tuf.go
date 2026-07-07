// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/foundriesio/update-server/clock"
	"github.com/foundriesio/update-server/storage/tuf"
	"golang.org/x/crypto/hkdf"
)

// ErrTufNotInitialized is returned when TUF operations are attempted before
// the TUF metadata and keys have been created with InitTuf.
var ErrTufNotInitialized = errors.New("TUF is not initialized")

// ErrTufAlreadyInitialized is returned by InitTuf when TUF data already exists.
var ErrTufAlreadyInitialized = errors.New("TUF is already initialized")

const (
	// tufKeysDir holds the encrypted role private keys.
	tufKeysDir = "keys"
	// rootJsonSuffix is the suffix for versioned root metadata files.
	rootJsonSuffix = ".root.json"

	// hkdfKeyEncSalt is the HKDF salt used to derive the key-file encryption key
	// from the HMAC secret.
	hkdfKeyEncSalt = "tuf-key-encryption-v1"
)

// tufRoles are the roles created and managed by the server.
var tufRoles = []tuf.RoleName{tuf.RoleRoot, tuf.RoleTargets, tuf.RoleSnapshot, tuf.RoleTimestamp}

// TufFsHandle manages TUF keys and metadata stored under <datadir>/tuf.
type TufFsHandle struct {
	baseFsHandle

	// auth provides access to the HMAC secret used to encrypt key files.
	auth AuthFsHandle

	updates updatesFsHandleWrap

	// RootExpiration is the validity period used for newly created root.json.
	RootExpiration time.Duration
	// TimestampExpiration is the validity period for timestamp metadata.
	TimestampExpiration time.Duration
	// TargetsExpiration is the validity period for targets metadata; snapshot
	// metadata uses the same value.
	TargetsExpiration time.Duration

	// signers holds the role private keys once LoadTuf has been called.
	signers map[tuf.RoleName]*tuf.Signer
}

func (h *TufFsHandle) init(root string, auth AuthFsHandle, updates updatesFsHandleWrap) {
	h.root = root
	h.auth = auth
	h.updates = updates
	h.RootExpiration = 20 * 365 * 24 * time.Hour
	h.TimestampExpiration = 7 * 24 * time.Hour
	h.TargetsExpiration = 90 * 24 * time.Hour
}

func (h TufFsHandle) keysDir() string {
	return filepath.Join(h.root, tufKeysDir)
}

func (h TufFsHandle) keyPath(role tuf.RoleName) string {
	return filepath.Join(h.keysDir(), string(role)+".key")
}

// isInitialized reports whether TUF metadata and keys have been created.
func (h TufFsHandle) isInitialized() bool {
	if _, err := os.Stat(h.keyPath(tuf.RoleRoot)); err != nil {
		return false
	}
	names, err := h.rootMetaNames()
	return err == nil && len(names) > 0
}

// InitTuf creates the TUF role keys (root, targets, snapshot, timestamp), an
// initial root.json, and stores the private keys encrypted with a key derived
// from the HMAC secret. It fails if TUF data already exists.
func (h TufFsHandle) InitTuf() error {
	if h.isInitialized() {
		return ErrTufAlreadyInitialized
	}

	hmacSecret, err := h.auth.GetHmacSecret()
	if err != nil {
		return fmt.Errorf("unable to read HMAC secret (run auth-init first): %w", err)
	} else if len(hmacSecret) == 0 {
		return fmt.Errorf("HMAC secret is empty; run auth-init first")
	}

	if err := os.MkdirAll(h.keysDir(), defaultDirAccess); err != nil {
		return fmt.Errorf("unable to create TUF keys directory: %w", err)
	}

	signers := make(map[tuf.RoleName]*tuf.Signer, len(tufRoles))
	for _, role := range tufRoles {
		signer, err := tuf.NewSigner()
		if err != nil {
			return fmt.Errorf("unable to generate %s key: %w", role, err)
		}
		if err := h.writeKey(hmacSecret, role, signer); err != nil {
			return err
		}
		signers[role] = signer
	}

	keys := make(map[string]tuf.AtsKey, len(signers))
	roles := make(map[tuf.RoleName]tuf.RootRole, len(signers))
	for _, role := range tufRoles {
		signer := signers[role]
		keys[signer.Id] = signer.PublicAtsKey()
		roles[role] = tuf.RootRole{KeyIDs: []string{signer.Id}, Threshold: 1}
	}
	root := tuf.AtsTufRoot{
		Signed: tuf.RootMeta{
			SignedCommon: tuf.SignedCommon{
				Type:    tuf.RoleRoot.TufType(),
				Expires: clock.Now().UTC().Add(h.RootExpiration).Truncate(time.Second),
				Version: 1,
			},
			ConsistentSnapshot: false,
			Keys:               keys,
			Roles:              roles,
		},
	}
	signed, err := signers[tuf.RoleRoot].Sign(root.Signed)
	if err != nil {
		return fmt.Errorf("unable to sign root metadata: %w", err)
	}
	root.Signatures = []tuf.Signature{signed}
	return h.writeRoot(root)
}

// LoadTuf loads and decrypts the role private keys into the handle. It returns
// ErrTufNotInitialized if TUF has not been initialized.
func (h *TufFsHandle) LoadTuf() error {
	if !h.isInitialized() {
		return ErrTufNotInitialized
	}

	hmacSecret, err := h.auth.GetHmacSecret()
	if err != nil {
		return fmt.Errorf("unable to read HMAC secret: %w", err)
	} else if len(hmacSecret) == 0 {
		return fmt.Errorf("HMAC secret is empty")
	}

	signers := make(map[tuf.RoleName]*tuf.Signer, len(tufRoles))
	for _, role := range tufRoles {
		signer, err := h.readKey(hmacSecret, role)
		if err != nil {
			return err
		}
		signers[role] = signer
	}
	h.signers = signers
	return nil
}

// GetRoots returns every root.json file on disk, unmarshalled and ordered by
// ascending version.
func (h TufFsHandle) GetRoots() ([]tuf.AtsTufRoot, error) {
	names, err := h.rootMetaNames()
	if err != nil {
		return nil, err
	}
	roots := make([]tuf.AtsTufRoot, 0, len(names))
	for _, name := range names {
		content, err := h.readFile(name, false)
		if err != nil {
			return nil, fmt.Errorf("unable to read %s: %w", name, err)
		}
		var root tuf.AtsTufRoot
		if err := json.Unmarshal([]byte(content), &root); err != nil {
			return nil, fmt.Errorf("unable to parse %s: %w", name, err)
		}
		roots = append(roots, root)
	}
	return roots, nil
}

// ReadRoot returns the raw JSON bytes of a root metadata file. A version <= 0
// returns the latest (highest version) root metadata. It returns an error that
// wraps os.ErrNotExist when the requested root does not exist.
func (h TufFsHandle) ReadRoot(version int) ([]byte, error) {
	name := strconv.Itoa(version) + rootJsonSuffix
	if version <= 0 {
		names, err := h.rootMetaNames()
		if err != nil {
			return nil, err
		}
		if len(names) == 0 {
			return nil, fmt.Errorf("no root metadata found: %w", os.ErrNotExist)
		}
		name = names[len(names)-1]
	}
	content, err := h.readFile(name, false)
	if err != nil {
		return nil, fmt.Errorf("unable to read %s: %w", name, err)
	}
	return []byte(content), nil
}

// ReadTufMeta reads and unmarshals a TUF metadata file from an update.
func (h TufFsHandle) ReadTufMeta(tag, update, name string, v any) error {
	content, err := h.updates.Tuf.ReadFile(tag, update, name)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(content), v); err != nil {
		return fmt.Errorf("unable to parse %s for tag %s update %s: %w", name, tag, update, err)
	}
	return nil
}

func (h TufFsHandle) Sign(role tuf.RoleName, v any) (tuf.Signature, error) {
	if !h.Enabled() {
		return tuf.Signature{}, fmt.Errorf("TUF signing not available: call LoadTuf first")
	}
	signer, ok := h.signers[role]
	if !ok {
		return tuf.Signature{}, fmt.Errorf("no signer loaded for TUF role %s", role)
	}
	return signer.Sign(v)
}

// Enabled reports whether TUF signing is available, i.e. LoadTuf has loaded the
// role keys.
func (h TufFsHandle) Enabled() bool {
	return len(h.signers) > 0
}

func (h TufFsHandle) WriteMeta(tufDir string, targets, snapshot, timestamp []byte) error {
	handle := baseFsHandle{root: tufDir}
	if err := handle.mkdirs(defaultDirAccess, true); err != nil {
		return fmt.Errorf("unable to create TUF directory: %w", err)
	}

	// link root metadata into the update directory
	names, err := h.rootMetaNames()
	if err != nil {
		return fmt.Errorf("unable to list root metadata: %w", err)
	}
	for _, name := range names {
		src := filepath.Join(h.root, name)
		dst := filepath.Join(tufDir, name)
		if err := os.Link(src, dst); err != nil {
			return fmt.Errorf("unable to link %s into update directory: %w", name, err)
		}
	}

	// Now write out the metadata
	for _, pair := range [][2]string{
		{TufTargetsFile, string(targets)},
		{TufSnapshotFile, string(snapshot)},
		{TufTimestampFile, string(timestamp)},
	} {
		if err := handle.writeFile(pair[0], pair[1], defaultFileAccess); err != nil {
			return fmt.Errorf("unable to write %s: %w", pair[0], err)
		}
	}

	return nil
}

func (h TufFsHandle) WriteTimestamp(tag, update string, ts []byte) error {
	return h.updates.Tuf.WriteFile(tag, update, "timestamp.json", string(ts))
}

// writeRoot persists a root metadata file as <version>.root.json.
func (h TufFsHandle) writeRoot(root tuf.AtsTufRoot) error {
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("unable to marshal root metadata: %w", err)
	}
	name := strconv.Itoa(root.Signed.Version) + rootJsonSuffix
	if err := h.mkdirs(defaultDirAccess, true); err != nil {
		return fmt.Errorf("unable to create TUF directory: %w", err)
	}
	if err := h.writeFile(name, string(data), defaultFileAccess); err != nil {
		return fmt.Errorf("unable to write %s: %w", name, err)
	}
	return nil
}

// rootMetaNames returns the names of all root metadata files, ordered by
// ascending version.
func (h TufFsHandle) rootMetaNames() ([]string, error) {
	entries, err := os.ReadDir(h.root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("unable to list TUF directory: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), rootJsonSuffix) {
			names = append(names, entry.Name())
		}
	}
	slices.SortFunc(names, func(a, b string) int {
		return rootVersion(a) - rootVersion(b)
	})
	return names, nil
}

// rootVersion extracts the integer version from a "<n>.root.json" name.
func rootVersion(name string) int {
	v, err := strconv.Atoi(strings.TrimSuffix(name, rootJsonSuffix))
	if err != nil {
		return 0
	}
	return v
}

// writeKey encrypts and stores a role's private key.
func (h TufFsHandle) writeKey(hmacSecret []byte, role tuf.RoleName, signer *tuf.Signer) error {
	plaintext, err := json.Marshal(signer.PrivateAtsKey())
	if err != nil {
		return fmt.Errorf("unable to marshal %s key: %w", role, err)
	}
	encKey, err := deriveKeyEncryptionKey(hmacSecret, role)
	if err != nil {
		return err
	}
	ciphertext, err := encryptBytes(encKey, plaintext)
	if err != nil {
		return fmt.Errorf("unable to encrypt %s key: %w", role, err)
	}
	keys := baseFsHandle{root: h.keysDir()}
	if err := keys.writeFile(string(role)+".key", string(ciphertext), secureFileAccess); err != nil {
		return fmt.Errorf("unable to store %s key: %w", role, err)
	}
	return nil
}

// readKey loads and decrypts a role's private key.
func (h TufFsHandle) readKey(hmacSecret []byte, role tuf.RoleName) (*tuf.Signer, error) {
	keys := baseFsHandle{root: h.keysDir()}
	ciphertext, err := keys.readFile(string(role)+".key", false)
	if err != nil {
		return nil, fmt.Errorf("unable to read %s key: %w", role, err)
	}
	encKey, err := deriveKeyEncryptionKey(hmacSecret, role)
	if err != nil {
		return nil, err
	}
	plaintext, err := decryptBytes(encKey, []byte(ciphertext))
	if err != nil {
		return nil, fmt.Errorf("unable to decrypt %s key: %w", role, err)
	}
	var key tuf.AtsKey
	if err := json.Unmarshal(plaintext, &key); err != nil {
		return nil, fmt.Errorf("unable to parse %s key: %w", role, err)
	}
	return tuf.SignerFromAtsKey(key)
}

// deriveKeyEncryptionKey derives a 32-byte AES key from the HMAC secret, scoped
// per role.
func deriveKeyEncryptionKey(hmacSecret []byte, role tuf.RoleName) ([]byte, error) {
	key := make([]byte, 32)
	r := hkdf.New(sha256.New, hmacSecret, []byte(hkdfKeyEncSalt), []byte(role))
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, fmt.Errorf("unable to derive key encryption key: %w", err)
	}
	return key, nil
}

// encryptBytes encrypts plaintext with AES-256-GCM, returning nonce||ciphertext.
func encryptBytes(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// decryptBytes reverses encryptBytes.
func decryptBytes(key, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ciphertext, nil)
}
