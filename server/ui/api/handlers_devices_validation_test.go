// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"net/http"
	"testing"

	"github.com/foundriesio/update-server/storage/users"
	"github.com/stretchr/testify/require"
)

func TestDeviceUUIDTraversal(t *testing.T) {
	for _, uuid := range []string{"", ".", "..", "../certs", `..\certs`, "%2e%2e", "device\n"} {
		t.Run(uuid, func(t *testing.T) {
			tc := NewTestClientWithCA(t, "test-ou")
			tc.u.AllowedScopes = users.ScopeDevicesC | users.ScopeDevicesD
			key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			require.NoError(t, err)
			csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
				Subject: pkix.Name{CommonName: uuid},
			}, key)
			require.NoError(t, err)
			response := tc.POST("/devices", http.StatusBadRequest, tc.marshalBody(DeviceCreateRequest{
				Uuid: uuid,
				Csr:  string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr})),
			}), "content-type", "application/json")
			require.Contains(t, string(response), "Invalid device UUID")
			require.Error(t, tc.api.DeviceCreate(uuid, "cert", nil))
			_, err = tc.gw.DeviceCreate(uuid, "cert")
			require.Error(t, err)
			device, err := tc.api.DeviceGet(uuid)
			require.NoError(t, err)
			require.Nil(t, device)
		})
	}
	tc := NewTestClient(t)
	tc.u.AllowedScopes = users.ScopeDevicesD
	for _, path := range []string{"..", ".", "%2e%2e", "..%2fcerts", "..%5ccerts"} {
		tc.DELETE("/devices/"+path, http.StatusBadRequest)
	}
}
