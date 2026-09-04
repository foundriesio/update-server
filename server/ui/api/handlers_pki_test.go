// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/foundriesio/update-server/storage"
)

func TestPkiFiles(t *testing.T) {
	tc := NewTestClientWithCA(t, "test-ou")

	casPem := []byte("-----BEGIN CERTIFICATE-----\ncas-bundle\n-----END CERTIFICATE-----\n")
	require.Nil(t, tc.fs.Certs.WriteFile(storage.CertsCasPemFile, casPem))

	for _, tt := range []struct {
		path string
		want map[string]string
	}{
		{"/v1/pki/cert?name=root.crt", mustRead(t, tc, storage.CertsRootPemFile)},
		{"/v1/pki/cert?name=tls.crt", mustRead(t, tc, storage.CertsTlsPemFile)},
		{"/v1/pki/cert?name=device-ca.crt", mustRead(t, tc, storage.CertsDeviceCaPemFile)},
		{"/v1/pki/cert?name=root.crt&name=tls.crt", mustRead(t, tc, storage.CertsRootPemFile, storage.CertsTlsPemFile)},
	} {
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		rec := tc.Do(req)
		require.Equal(t, http.StatusOK, rec.Code, tt.path)
		var respMap map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &respMap); err != nil {
			t.Fatalf("Failed to parse response JSON for %s: %v", tt.path, err)
		}
		assert.Equal(t, tt.want, respMap, tt.path)
	}
}

func TestPkiDeviceCaMissing(t *testing.T) {
	// No CA configured, so device-ca.crt does not exist on disk.
	tc := NewTestClient(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/pki/cert?name=device-ca.crt", nil)
	rec := tc.Do(req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, `{"device-ca.crt":""}`, strings.TrimSpace(rec.Body.String()))
}

func mustRead(t *testing.T, tc *testClient, name ...string) map[string]string {
	resp := make(map[string]string, len(name))
	for _, n := range name {
		buf, err := tc.fs.Certs.ReadFile(n)
		require.Nil(t, err)
		resp[n] = string(buf)
	}
	return resp
}
