// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package tuf

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"fmt"

	"github.com/secure-systems-lab/go-securesystemslib/cjson"
)

// tufKeyTypeEd25519 is the keytype value used by ota-tuf for ed25519 keys.
const tufKeyTypeEd25519 = "ED25519"

// tufSigner holds a loaded private key along with its TUF key id. The server
// uses online ed25519 keys for all roles.
type Signer struct {
	Id      string
	keyType string
	private ed25519.PrivateKey
	public  ed25519.PublicKey
}

// tufKeyID computes the key id the way ota-tuf/garage-sign does: the hex
// encoded sha256 of the key's canonical (PKIX/DER) public encoding.
func tufKeyID(pub ed25519.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", fmt.Errorf("unable to marshal public key: %w", err)
	}
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:]), nil
}

// NewSigner generates a fresh ed25519 signer.
func NewSigner() (*Signer, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("unable to generate ed25519 key: %w", err)
	}
	id, err := tufKeyID(pub)
	if err != nil {
		return nil, err
	}
	return &Signer{Id: id, keyType: tufKeyTypeEd25519, private: priv, public: pub}, nil
}

// SignerFromAtsKey reconstructs a signer from a stored AtsKey (private key).
func SignerFromAtsKey(key AtsKey) (*Signer, error) {
	if key.KeyType != tufKeyTypeEd25519 {
		return nil, fmt.Errorf("unsupported TUF key type: %s", key.KeyType)
	}
	if key.KeyValue.Private == "" {
		return nil, fmt.Errorf("TUF key is missing private material")
	}
	seed, err := hex.DecodeString(key.KeyValue.Private)
	if err != nil {
		return nil, fmt.Errorf("invalid ed25519 private key encoding: %w", err)
	}
	var priv ed25519.PrivateKey
	switch len(seed) {
	case ed25519.SeedSize:
		priv = ed25519.NewKeyFromSeed(seed)
	case ed25519.PrivateKeySize:
		priv = ed25519.PrivateKey(seed)
	default:
		return nil, fmt.Errorf("invalid ed25519 private key size: %d", len(seed))
	}
	pub := priv.Public().(ed25519.PublicKey)
	id, err := tufKeyID(pub)
	if err != nil {
		return nil, err
	}
	return &Signer{Id: id, keyType: tufKeyTypeEd25519, private: priv, public: pub}, nil
}

// privateAtsKey returns the AtsKey representation of the signer including the
// private key material (hex encoded seed).
func (s *Signer) PrivateAtsKey() AtsKey {
	return AtsKey{
		KeyType: s.keyType,
		KeyValue: AtsKeyVal{
			Private: hex.EncodeToString(s.private.Seed()),
			Public:  hex.EncodeToString(s.public),
		},
	}
}

// publicAtsKey returns the AtsKey representation of the signer with only the
// public key material, as embedded in root metadata.
func (s *Signer) PublicAtsKey() AtsKey {
	return AtsKey{
		KeyType:  s.keyType,
		KeyValue: AtsKeyVal{Public: hex.EncodeToString(s.public)},
	}
}

// sign signs the canonical JSON of signed and returns a Signature.
func (s *Signer) Sign(signed any) (Signature, error) {
	msg, err := cjson.EncodeCanonical(signed)
	if err != nil {
		return Signature{}, fmt.Errorf("unable to marshal canonical JSON: %w", err)
	}
	sig := ed25519.Sign(s.private, msg)
	return Signature{KeyID: s.Id, Method: SigEd25519, Signature: sig}, nil
}
