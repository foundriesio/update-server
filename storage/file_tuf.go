// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	tufSecretFile  = "tuf.secret"
	tufRootKeyFile = "root.key"
	tufTsKeyFile   = "timestamp.key"
	tufSnapKeyFile = "snapshot.key"
	tufTgtsKeyFile = "targets.key"

	tufKeyTypeEd25519   = "ED25519"
	tufSigMethodEd25519 = "ed25519"

	tufRoleRoot      = "root"
	tufRoleTimestamp = "timestamp"
	tufRoleSnapshot  = "snapshot"
	tufRoleTargets   = "targets"
)

// TUF data structures matching the fioctl/FoundriesIO ATS format.

type TufKeyVal struct {
	Public  string `json:"public,omitempty"`
	Private string `json:"private,omitempty"`
}

type TufKey struct {
	KeyType string    `json:"keytype"`
	KeyVal  TufKeyVal `json:"keyval"`
}

type TufRootRole struct {
	KeyIDs    []string `json:"keyids"`
	Threshold int      `json:"threshold"`
}

type TufSignature struct {
	KeyID  string `json:"keyid"`
	Method string `json:"method"`
	Sig    string `json:"sig"`
}

type TufRootMeta struct {
	Type               string                  `json:"_type"`
	Expires            time.Time               `json:"expires"`
	Version            int                     `json:"version"`
	ConsistentSnapshot bool                    `json:"consistent_snapshot"`
	Keys               map[string]TufKey       `json:"keys"`
	Roles              map[string]*TufRootRole `json:"roles"`
}

type TufRoot struct {
	Signatures []TufSignature `json:"signatures"`
	Signed     TufRootMeta    `json:"signed"`
}

// TufFsHandle manages TUF key material and root metadata on disk.
type TufFsHandle struct {
	baseFsHandle
	aesKey       []byte
	RootKey      ed25519.PrivateKey
	TimestampKey ed25519.PrivateKey
	SnapshotKey  ed25519.PrivateKey
	TargetsKey   ed25519.PrivateKey
}

// encryptedKey is the on-disk JSON format for an encrypted ed25519 seed.
type encryptedKey struct {
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

func tufKeyID(pub ed25519.PublicKey) (string, error) {
	pkixBytes, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", fmt.Errorf("marshaling public key: %w", err)
	}
	sum := sha256.Sum256(pkixBytes)
	return hex.EncodeToString(sum[:]), nil
}

func tufPublicKeyHex(pub ed25519.PublicKey) string {
	return hex.EncodeToString([]byte(pub))
}

func (h TufFsHandle) encryptSeed(seed []byte) ([]byte, error) {
	block, err := aes.NewCipher(h.aesKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	ct := gcm.Seal(nil, nonce, seed, nil)
	ek := encryptedKey{
		Nonce:      hex.EncodeToString(nonce),
		Ciphertext: hex.EncodeToString(ct),
	}
	return json.Marshal(ek)
}

func (h TufFsHandle) decryptSeed(data []byte) ([]byte, error) {
	var ek encryptedKey
	if err := json.Unmarshal(data, &ek); err != nil {
		return nil, fmt.Errorf("parsing encrypted key: %w", err)
	}
	nonce, err := hex.DecodeString(ek.Nonce)
	if err != nil {
		return nil, fmt.Errorf("decoding nonce: %w", err)
	}
	ct, err := hex.DecodeString(ek.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decoding ciphertext: %w", err)
	}
	block, err := aes.NewCipher(h.aesKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ct, nil)
}

func (h TufFsHandle) saveKey(filename string, priv ed25519.PrivateKey) error {
	data, err := h.encryptSeed(priv.Seed())
	if err != nil {
		return fmt.Errorf("encrypting %s: %w", filename, err)
	}
	return h.writeFile(filename, string(data), secureFileAccess)
}

func (h TufFsHandle) loadKey(filename string) (ed25519.PrivateKey, error) {
	raw, err := h.readFile(filename, false)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", filename, err)
	}
	seed, err := h.decryptSeed([]byte(raw))
	if err != nil {
		return nil, fmt.Errorf("decrypting %s: %w", filename, err)
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

func (h TufFsHandle) signRoot(meta TufRootMeta, priv ed25519.PrivateKey) (TufRoot, error) {
	pub := priv.Public().(ed25519.PublicKey)
	keyID, err := tufKeyID(pub)
	if err != nil {
		return TufRoot{}, err
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return TufRoot{}, fmt.Errorf("marshaling root meta: %w", err)
	}
	sigBytes := ed25519.Sign(priv, metaBytes)
	return TufRoot{
		Signed: meta,
		Signatures: []TufSignature{{
			KeyID:  keyID,
			Method: tufSigMethodEd25519,
			Sig:    hex.EncodeToString(sigBytes),
		}},
	}, nil
}

// InitTuf initializes TUF key material and writes 1.root.json. Fails if already initialized.
func InitTuf(fs *FsHandle) (*TufFsHandle, error) {
	h := &TufFsHandle{baseFsHandle: fs.Tuf.baseFsHandle}

	if _, err := h.readFile("1.root.json", false); err == nil {
		return nil, errors.New("TUF already initialized")
	}

	// Generate and persist a dedicated 32-byte AES-256 secret.
	aesKey := make([]byte, 32)
	if _, err := rand.Read(aesKey); err != nil {
		return nil, fmt.Errorf("generating TUF secret: %w", err)
	}
	if err := h.writeFile(tufSecretFile, string(aesKey), secureFileAccess); err != nil {
		return nil, fmt.Errorf("storing TUF secret: %w", err)
	}
	h.aesKey = aesKey

	// Generate four ed25519 keypairs.
	type roleKey struct {
		name string
		file string
		pub  ed25519.PublicKey
		priv ed25519.PrivateKey
	}
	roles := []roleKey{
		{name: tufRoleRoot, file: tufRootKeyFile},
		{name: tufRoleTimestamp, file: tufTsKeyFile},
		{name: tufRoleSnapshot, file: tufSnapKeyFile},
		{name: tufRoleTargets, file: tufTgtsKeyFile},
	}
	for i := range roles {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generating %s key: %w", roles[i].name, err)
		}
		if err = h.saveKey(roles[i].file, priv); err != nil {
			return nil, err
		}
		roles[i].pub = pub
		roles[i].priv = priv
	}
	h.RootKey = roles[0].priv
	h.TimestampKey = roles[1].priv
	h.SnapshotKey = roles[2].priv
	h.TargetsKey = roles[3].priv

	// Build the root metadata.
	keys := make(map[string]TufKey, len(roles))
	rolesMeta := make(map[string]*TufRootRole, len(roles))
	for _, r := range roles {
		keyID, err := tufKeyID(r.pub)
		if err != nil {
			return nil, err
		}
		keys[keyID] = TufKey{
			KeyType: tufKeyTypeEd25519,
			KeyVal:  TufKeyVal{Public: tufPublicKeyHex(r.pub)},
		}
		rolesMeta[r.name] = &TufRootRole{
			KeyIDs:    []string{keyID},
			Threshold: 1,
		}
	}

	meta := TufRootMeta{
		Type:               "Root",
		Version:            1,
		Expires:            time.Now().Add(10 * 365 * 24 * time.Hour),
		ConsistentSnapshot: false,
		Keys:               keys,
		Roles:              rolesMeta,
	}
	root, err := h.signRoot(meta, h.RootKey)
	if err != nil {
		return nil, fmt.Errorf("signing root: %w", err)
	}

	rootBytes, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling root.json: %w", err)
	}
	if err = h.writeFile("1.root.json", string(rootBytes), secureFileAccess); err != nil {
		return nil, fmt.Errorf("writing 1.root.json: %w", err)
	}
	return h, nil
}

// LoadTuf loads existing TUF key material from disk. Returns an error if TUF is not initialized.
func LoadTuf(fs *FsHandle) (*TufFsHandle, error) {
	h := &TufFsHandle{baseFsHandle: fs.Tuf.baseFsHandle}

	if _, err := h.readFile("1.root.json", false); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errors.New("TUF not initialized")
		}
		return nil, fmt.Errorf("checking TUF state: %w", err)
	}

	secret, err := h.readFile(tufSecretFile, false)
	if err != nil {
		return nil, fmt.Errorf("reading TUF secret: %w", err)
	}
	h.aesKey = []byte(secret)

	if h.RootKey, err = h.loadKey(tufRootKeyFile); err != nil {
		return nil, err
	}
	if h.TimestampKey, err = h.loadKey(tufTsKeyFile); err != nil {
		return nil, err
	}
	if h.SnapshotKey, err = h.loadKey(tufSnapKeyFile); err != nil {
		return nil, err
	}
	if h.TargetsKey, err = h.loadKey(tufTgtsKeyFile); err != nil {
		return nil, err
	}
	return h, nil
}

// GetRootJSON returns the raw JSON bytes of the root at the given version.
// Pass version 0 to get the latest version.
func (h *TufFsHandle) GetRootJSON(version int) (string, error) {
	if version == 0 {
		files, err := h.matchFiles("", false)
		if err != nil {
			return "", fmt.Errorf("listing TUF files: %w", err)
		}
		for _, f := range files {
			if !strings.HasSuffix(f, ".root.json") {
				continue
			}
			ver, err := strconv.Atoi(strings.TrimSuffix(f, ".root.json"))
			if err != nil {
				continue
			}
			if ver > version {
				version = ver
			}
		}
		if version == 0 {
			return "", errors.New("no root.json files found")
		}
	}
	name := fmt.Sprintf("%d.root.json", version)
	data, err := h.readFile(name, false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("root version %d not found", version)
		}
		return "", fmt.Errorf("reading %s: %w", name, err)
	}
	return data, nil
}

// GetRoots returns all versioned root.json files, sorted by ascending version number.
func (h *TufFsHandle) GetRoots() ([]*TufRoot, error) {
	files, err := h.matchFiles("", false)
	if err != nil {
		return nil, fmt.Errorf("listing TUF files: %w", err)
	}

	type versioned struct {
		ver  int
		name string
	}
	var roots []versioned
	for _, f := range files {
		if !strings.HasSuffix(f, ".root.json") {
			continue
		}
		prefix := strings.TrimSuffix(f, ".root.json")
		ver, err := strconv.Atoi(prefix)
		if err != nil {
			continue
		}
		roots = append(roots, versioned{ver: ver, name: f})
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].ver < roots[j].ver })

	result := make([]*TufRoot, 0, len(roots))
	for _, r := range roots {
		path := filepath.Join(h.root, r.name)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", r.name, err)
		}
		var root TufRoot
		if err = json.Unmarshal(data, &root); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", r.name, err)
		}
		result = append(result, &root)
	}
	return result, nil
}
