// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package tuf

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"

	"github.com/secure-systems-lab/go-securesystemslib/cjson"
)

// tufKeyTypeRSA is the keytype value used by ota-tuf for RSA keys.
const tufKeyTypeRSA = "RSA"

// ImportSigner wraps an offline private key (RSA or ed25519) imported from an
// external tool such as fioctl/garage-sign. It is used to co-sign a newly
// generated root metadata during migration so that the new root chains from
// the previously trusted (imported) root.
type ImportSigner struct {
	Id     string
	method SigAlgorithm
	signer crypto.Signer
	opts   crypto.SignerOpts
}

// ImportSignerFromAtsKey builds an ImportSigner from a private AtsKey. Both the
// ota-tuf RSA (PEM PKCS#1) and ed25519 (hex) private key representations are
// supported.
func ImportSignerFromAtsKey(key AtsKey) (*ImportSigner, error) {
	if key.KeyValue.Private == "" {
		return nil, fmt.Errorf("TUF key is missing private material")
	}
	switch key.KeyType {
	case tufKeyTypeEd25519:
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
		id, err := tufKeyID(priv.Public().(ed25519.PublicKey))
		if err != nil {
			return nil, err
		}
		return &ImportSigner{Id: id, method: SigEd25519, signer: priv, opts: crypto.Hash(0)}, nil
	case tufKeyTypeRSA:
		block, _ := pem.Decode([]byte(key.KeyValue.Private))
		if block == nil {
			return nil, fmt.Errorf("unable to decode RSA private key PEM")
		}
		priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("unable to parse RSA private key: %w", err)
		}
		id, err := rsaKeyID(&priv.PublicKey)
		if err != nil {
			return nil, err
		}
		return &ImportSigner{
			Id:     id,
			method: SigRsaPssSha256,
			signer: priv,
			// ota-tuf/garage-sign sign RSA with RSASSA-PSS, SHA-256 and a salt
			// length equal to the hash length.
			opts: &rsa.PSSOptions{SaltLength: 32, Hash: crypto.SHA256},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported TUF key type: %s", key.KeyType)
	}
}

// rsaKeyID computes the key id for an RSA public key the same way ota-tuf does:
// the hex encoded sha256 of the key's canonical (PKIX/DER) public encoding.
func rsaKeyID(pub *rsa.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", fmt.Errorf("unable to marshal public key: %w", err)
	}
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:]), nil
}

// Sign signs the canonical JSON of signed with the imported key and returns a
// SignedMeta carrying the signature along with the sha256 hash and length of
// the signed bytes.
func (s *ImportSigner) Sign(signed any) (Signature, error) {
	msg, err := cjson.EncodeCanonical(signed)
	if err != nil {
		return Signature{}, fmt.Errorf("unable to marshal canonical JSON: %w", err)
	}
	digest := msg
	if h := s.opts.HashFunc(); h != crypto.Hash(0) {
		// Golang expects the caller to hash the message for signing methods
		// that use a hash (e.g. RSASSA-PSS).
		hasher := h.New()
		hasher.Write(msg)
		digest = hasher.Sum(nil)
	}
	sig, err := s.signer.Sign(rand.Reader, digest, s.opts)
	if err != nil {
		return Signature{}, fmt.Errorf("unable to sign metadata: %w", err)
	}
	return Signature{KeyID: s.Id, Method: s.method, Signature: sig}, nil
}
