// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"encoding/pem"
	"fmt"
	"os"
	"strings"
)

func PemBytesToObject[T any](pemBytes []byte, parse func([]byte) (T, error)) (T, error) {
	first, rest := pem.Decode(pemBytes)
	if first == nil || len(strings.TrimSpace(string(rest))) > 0 {
		var zero T
		return zero, fmt.Errorf("malformed PEM data")
	}

	return parse(first.Bytes)
}

func LoadPemFile[T any](path string, parse func([]byte) (T, error)) (T, error) {
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("unable to read %s: %w", path, err)
	}
	obj, err := PemBytesToObject(pemBytes, parse)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("%w for %s", err, path)
	}
	return obj, nil
}
