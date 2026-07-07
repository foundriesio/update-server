// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/foundriesio/update-server/clock"
	"github.com/foundriesio/update-server/storage/tuf"
)

// importedRoot holds the imported root metadata along with its raw bytes. The
// raw bytes are stored verbatim so the original signatures remain verifiable.
type importedRoot struct {
	raw  []byte
	root tuf.AtsTufRoot
}

// ImportTuf initializes TUF for this server by migrating from an existing
// fioctl/ota-tuf setup.
//
// rootJSONs are the raw bytes of every known root.json version. candidateKeys
// are the private keys extracted from a fioctl offline keys tarball,
// including the offline private key(s) for the root role. Every imported
// root.json is stored verbatim, fresh online keys are generated for every role,
// and a new root.json (version = highest imported version + 1) is created and
// signed by both the imported (old) root key(s) and the newly generated root
// key so that clients can verify the chain of trust from the previously
// trusted root.
//
// It fails if TUF data already exists.
func (h TufFsHandle) ImportTuf(rootJSONs [][]byte, candidateKeys []tuf.AtsKey) error {
	if h.isInitialized() {
		return ErrTufAlreadyInitialized
	}

	if len(rootJSONs) == 0 {
		return fmt.Errorf("no root metadata was provided to import")
	}
	roots := make([]importedRoot, 0, len(rootJSONs))
	for _, raw := range rootJSONs {
		var root tuf.AtsTufRoot
		if err := json.Unmarshal(raw, &root); err != nil {
			return fmt.Errorf("unable to parse root metadata: %w", err)
		}
		if len(root.Signed.Roles) == 0 {
			return fmt.Errorf("provided metadata does not look like a TUF root.json")
		}
		roots = append(roots, importedRoot{raw: raw, root: root})
	}

	return h.importTuf(roots, candidateKeys)
}

// importTuf performs the actual import once the root metadata and keys have
// been parsed.
func (h TufFsHandle) importTuf(roots []importedRoot, candidateKeys []tuf.AtsKey) error {
	hmacSecret, err := h.auth.GetHmacSecret()
	if err != nil {
		return fmt.Errorf("unable to read HMAC secret (run auth-init first): %w", err)
	} else if len(hmacSecret) == 0 {
		return fmt.Errorf("HMAC secret is empty; run auth-init first")
	}

	// The highest-version imported root is the trust anchor to chain from.
	slices.SortFunc(roots, func(a, b importedRoot) int {
		return a.root.Signed.Version - b.root.Signed.Version
	})
	base := roots[len(roots)-1]

	// ensure we have all the root.json versions
	for idx, root := range roots {
		if idx+1 != root.root.Signed.Version {
			return fmt.Errorf("missing %d.root.json version", idx+1)
		}
	}

	rootThresh := base.root.Signed.Roles[tuf.RoleRoot].Threshold
	if rootThresh > 1 {
		return fmt.Errorf("unable to import TUF root. The signature threshold for the root role must be 1. Current value is: %d", rootThresh)
	}

	oldRootRole, ok := base.root.Signed.Roles[tuf.RoleRoot]
	if !ok {
		return fmt.Errorf("imported root.json (version %d) has no root role", base.root.Signed.Version)
	}

	// Find the offline root signer(s) matching the imported root role key ids.
	oldSigners, err := matchRootSigners(oldRootRole.KeyIDs, candidateKeys)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(h.keysDir(), defaultDirAccess); err != nil {
		return fmt.Errorf("unable to create TUF keys directory: %w", err)
	}

	// Generate fresh online keys for every role owned by this server.
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
	newRoot := tuf.AtsTufRoot{
		Signed: tuf.RootMeta{
			SignedCommon: tuf.SignedCommon{
				Type:    tuf.RoleRoot.TufType(),
				Expires: clock.Now().UTC().Add(h.RootExpiration).Truncate(time.Second),
				Version: base.root.Signed.Version + 1,
			},
			ConsistentSnapshot: false,
			Keys:               keys,
			Roles:              roles,
		},
	}

	// The new root must be signed by the new root key and one matching old
	// root key so it chains from the previously trusted root.
	newSigned, err := signers[tuf.RoleRoot].Sign(newRoot.Signed)
	if err != nil {
		return fmt.Errorf("unable to sign new root metadata: %w", err)
	}
	signatures := []tuf.Signature{newSigned}
	for _, old := range oldSigners {
		oldSigned, err := old.Sign(newRoot.Signed)
		if err != nil {
			return fmt.Errorf("unable to sign new root metadata with imported key %s: %w", old.Id, err)
		}
		signatures = append(signatures, oldSigned)
	}
	newRoot.Signatures = signatures

	// Persist every imported root verbatim, then the newly generated root.
	if err := h.mkdirs(defaultDirAccess, true); err != nil {
		return fmt.Errorf("unable to create TUF directory: %w", err)
	}
	for _, imp := range roots {
		name := strconv.Itoa(imp.root.Signed.Version) + rootJsonSuffix
		if err := h.writeFile(name, string(imp.raw), defaultFileAccess); err != nil {
			return fmt.Errorf("unable to write %s: %w", name, err)
		}
	}
	return h.writeRoot(newRoot)
}

// matchRootSigners returns an ImportSigner for every candidate private key
// whose key id is listed in keyIDs. It errors if no matching key is found.
func matchRootSigners(keyIDs []string, candidateKeys []tuf.AtsKey) ([]*tuf.ImportSigner, error) {
	wanted := make(map[string]bool, len(keyIDs))
	for _, id := range keyIDs {
		wanted[id] = true
	}

	var signers []*tuf.ImportSigner
	for _, key := range candidateKeys {
		signer, err := tuf.ImportSignerFromAtsKey(key)
		if err != nil {
			// Not a usable signing key (e.g. public-only or unsupported type).
			continue
		}
		if wanted[signer.Id] {
			signers = append(signers, signer)
			delete(wanted, signer.Id)
		}
	}
	if len(signers) == 0 {
		return nil, fmt.Errorf(
			"keys archive does not contain a private key for any of the root role key ids: %v",
			keyIDs,
		)
	}
	return signers, nil
}
