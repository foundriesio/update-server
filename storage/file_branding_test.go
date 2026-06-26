// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadBrandingConfigAbsentReturnsNil(t *testing.T) {
	c := FsConfig(t.TempDir())
	data, err := c.ReadBrandingConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data != nil {
		t.Errorf("data = %q, want nil when branding.json absent", data)
	}
}

func TestReadBrandingConfigReturnsContents(t *testing.T) {
	dir := t.TempDir()
	c := FsConfig(dir)
	if err := os.MkdirAll(c.BrandingDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	want := []byte(`{"title":"Acme"}`)
	if err := os.WriteFile(filepath.Join(c.BrandingDir(), BrandingConfigFile), want, 0o640); err != nil {
		t.Fatal(err)
	}
	got, err := c.ReadBrandingConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("got %q, want %q", got, want)
	}
}
