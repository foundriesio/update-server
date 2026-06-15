// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"fmt"
	"net/http"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/foundriesio/update-server/context"
	"github.com/foundriesio/update-server/storage"
)

func TestServe(t *testing.T) {
	tmpDir := t.TempDir()
	common := CommonArgs{DataDir: tmpDir}
	fs, err := storage.NewFs(common.DataDir)
	require.Nil(t, err)
	require.Nil(t, fs.Auth.InitHmacSecret())
	_, err = storage.InitTuf(fs)
	require.Nil(t, err)
	authConfig := storage.AuthConfig{
		Type: "noauth",
	}
	require.Nil(t, fs.Auth.SaveAuthConfig(authConfig))
	apiAddress := ""
	gatewayAddress := ""
	wait := make(chan bool)
	server := ServeCmd{
		startedCb: func(apiAddr, gwAddr string) {
			apiAddress = apiAddr
			gatewayAddress = gwAddr
			wait <- true
		},
		UiAddr:      "127.0.0.1:0",
		GatewayAddr: "127.0.0.1:0",
	}

	log, err := context.InitLogger("debug")
	require.Nil(t, err)
	common.ctx = context.CtxWithLog(context.Background(), log)

	csr := CsrCmd{
		DnsName: "example.com",
		Factory: "example",
	}

	err = csr.Run(common)
	require.Nil(t, err)
	caKeyFile, caFile := createSelfSignedRoot(t, fs)
	sign := CsrSignCmd{
		CaKey:      caKeyFile,
		CaCert:     caFile,
		ExpiryDays: 10,
	}
	require.Nil(t, sign.Run(common))
	// create an empty ca file to make the server happy. no client will be able to handshake with it
	require.Nil(t, fs.Certs.WriteFile(storage.CertsCasPemFile, []byte{}))

	go func() {
		if err = server.Run(common); err != nil {
			// Unblock main thread and check an error over there
			wait <- false
		}
	}()
	<-wait
	require.Nil(t, err)

	r, err := http.Get(fmt.Sprintf("http://%s/doesnotexist", apiAddress))
	require.Nil(t, err)
	require.Equal(t, http.StatusNotFound, r.StatusCode)
	require.Equal(t, 12, len(r.Header.Get("X-Request-Id")))

	_, err = http.Get(fmt.Sprintf("https://%s/doesnotexist", gatewayAddress))
	require.NotNil(t, err)
	require.Contains(t, err.Error(), "failed to verify certificate")

	require.Nil(t, syscall.Kill(syscall.Getpid(), syscall.SIGINT))
}
