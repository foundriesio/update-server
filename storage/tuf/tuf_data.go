// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package tuf

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"
)

// This file defines the TUF (The Update Framework) metadata structures used by
// this project. The on-disk format follows the one produced by Foundries.io
// ota-tuf / garage-sign (and consumed by libaktualizr), which differs slightly
// from the standard go-tuf format:
//
//   - The "_type" value of the signed metadata is the capitalized role name
//     ("Root", "Targets", "Snapshot", "Timestamp").
//   - The "roles" map in root metadata is keyed by the lower-case role name.
//   - Signatures carry the raw signature bytes which are base64 encoded in JSON.
//   - Keys are represented by the AtsKey structure where ed25519 public and
//     private material is hex encoded.
//   - snapshot/timestamp "meta" entries only reference a version, not hashes.
//
// These structures are intentionally self-contained: the project does not
// import TUF data types from any external library.

// RoleName is the canonical (lower-case) name of a TUF role.
type RoleName string

const (
	RoleRoot      RoleName = "root"
	RoleTargets   RoleName = "targets"
	RoleSnapshot  RoleName = "snapshot"
	RoleTimestamp RoleName = "timestamp"
)

// tufType returns the "_type" value used in signed metadata for the role.
func (r RoleName) TufType() string {
	switch r {
	case RoleRoot:
		return "Root"
	case RoleTargets:
		return "Targets"
	case RoleSnapshot:
		return "Snapshot"
	case RoleTimestamp:
		return "Timestamp"
	default:
		return string(r)
	}
}

// SigAlgorithm is the signing method recorded in a Signature.
type SigAlgorithm string

const (
	SigEd25519      SigAlgorithm = "ed25519"
	SigRsaPssSha256 SigAlgorithm = "rsassa-pss-sha256"
)

// SignedCommon contains the fields common to the "signed" component of all
// TUF metadata files.
type SignedCommon struct {
	Type    string    `json:"_type"`
	Expires time.Time `json:"expires"`
	Version int       `json:"version"`
}

// Signature is a signature over the canonical JSON of a metadata's "signed"
// component. The raw signature bytes are base64 encoded in JSON.
type Signature struct {
	KeyID     string       `json:"keyid"`
	Method    SigAlgorithm `json:"method"`
	Signature []byte       `json:"sig"`
}

// AtsKeyVal holds the (hex encoded) public and/or private key material.
type AtsKeyVal struct {
	Public  string `json:"public,omitempty"`
	Private string `json:"private,omitempty"`
}

// AtsKey is the ota-tuf representation of a key.
type AtsKey struct {
	KeyType  string    `json:"keytype"`
	KeyValue AtsKeyVal `json:"keyval"`
}

// RootRole describes the keys and threshold for a role within root metadata.
type RootRole struct {
	KeyIDs    []string `json:"keyids"`
	Threshold int      `json:"threshold"`
}

// RootMeta is the "signed" component of a root.json file.
type RootMeta struct {
	SignedCommon
	ConsistentSnapshot bool                  `json:"consistent_snapshot"`
	Keys               map[string]AtsKey     `json:"keys"`
	Roles              map[RoleName]RootRole `json:"roles"`
}

// AtsTufRoot is a full root.json file.
type AtsTufRoot struct {
	Signatures []Signature `json:"signatures"`
	Signed     RootMeta    `json:"signed"`
}

// HexBytes is a byte slice that is hex encoded in JSON. TUF hashes use this
// representation (unlike signatures, which are base64 encoded).
type HexBytes []byte

func (b HexBytes) MarshalJSON() ([]byte, error) {
	return json.Marshal(hex.EncodeToString(b))
}

func (b *HexBytes) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	decoded, err := hex.DecodeString(s)
	if err != nil {
		return fmt.Errorf("invalid hex encoded bytes: %w", err)
	}
	*b = decoded
	return nil
}

// Hashes maps a hash algorithm name (e.g. "sha256") to a hex encoded digest.
type Hashes map[string]HexBytes

// TargetFileMeta describes a single target file in targets metadata.
type TargetFileMeta struct {
	Length int64           `json:"length"`
	Hashes Hashes          `json:"hashes"`
	Custom json.RawMessage `json:"custom,omitempty"`
}

// TargetFiles maps a target name to its metadata.
type TargetFiles map[string]TargetFileMeta

// TargetsMeta is the "signed" component of a targets.json file.
type TargetsMeta struct {
	SignedCommon
	Targets TargetFiles `json:"targets"`
}

// AtsTufTargets is a full targets.json file.
type AtsTufTargets struct {
	Signatures []Signature `json:"signatures"`
	Signed     TargetsMeta `json:"signed"`
}

func (t AtsTufTargets) GetLatestTargetVersion() int {
	maxVersion := 0
	for name, target := range t.Signed.Targets {
		var custom struct {
			Version string `json:"version"`
		}
		if err := json.Unmarshal(target.Custom, &custom); err != nil {
			slog.Warn("Unable to parse target custom", "target-name", name, "error", err)
			continue
		}
		if n, err := strconv.Atoi(custom.Version); err == nil && n > maxVersion {
			maxVersion = n
		}
	}
	return maxVersion
}

// MetaItem references a version of another metadata file. The ota-tuf format
// for snapshot and timestamp metadata only records the version.
type MetaItem struct {
	Version int `json:"version"`
}

// SnapshotMeta is the "signed" component of a snapshot.json file.
type SnapshotMeta struct {
	SignedCommon
	Meta map[string]MetaItem `json:"meta"`
}

// AtsTufSnapshot is a full snapshot.json file.
type AtsTufSnapshot struct {
	Signatures []Signature  `json:"signatures"`
	Signed     SnapshotMeta `json:"signed"`
}

// TimestampMeta is the "signed" component of a timestamp.json file.
type TimestampMeta struct {
	SignedCommon
	Meta map[string]MetaItem `json:"meta"`
}

// AtsTufTimestamp is a full timestamp.json file.
type AtsTufTimestamp struct {
	Signatures []Signature   `json:"signatures"`
	Signed     TimestampMeta `json:"signed"`
}
