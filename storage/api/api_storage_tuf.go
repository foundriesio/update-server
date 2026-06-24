// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

// GetTufRoot returns the raw JSON bytes of a TUF root metadata file. A version
// of 0 (or less) returns the latest root metadata. The returned error wraps
// os.ErrNotExist when the requested root does not exist.
func (s Storage) GetTufRoot(version int) ([]byte, error) {
	return s.fs.Tuf.ReadRoot(version)
}
