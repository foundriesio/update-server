// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"fmt"
	"net/http"
	"sync/atomic"
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
	require.NoError(t, err)
	require.NoError(t, fs.Auth.InitHmacSecret())
	require.NoError(t, fs.Tuf.InitTuf())
	authConfig := storage.AuthConfig{
		Type: "noauth",
	}
	require.NoError(t, fs.Auth.SaveAuthConfig(authConfig))
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
	require.NoError(t, err)
	common.ctx = context.CtxWithLog(context.Background(), log)

	csr := CsrCmd{
		DnsName: "example.com",
		Factory: "example",
	}

	err = csr.Run(common)
	require.NoError(t, err)
	caKeyFile, caFile := createSelfSignedRoot(t, fs)
	sign := CsrSignCmd{
		CaKey:      caKeyFile,
		CaCert:     caFile,
		ExpiryDays: 10,
	}
	require.NoError(t, sign.Run(common))
	// create an empty ca file to make the server happy. no client will be able to handshake with it
	require.NoError(t, fs.Certs.WriteFile(storage.CertsCasPemFile, []byte{}))

	// Use an atomic for serveErr so errors from server.Run() can be saved while the test goroutine is reading it.
	var serveErr atomic.Value
	go func() {
		if err := server.Run(common); err != nil {
			serveErr.Store(err)
			// Unblock main thread and check an error over there
			wait <- false
		}
	}()
	<-wait
	require.Nil(t, serveErr.Load())

	r, err := http.Get(fmt.Sprintf("http://%s/doesnotexist", apiAddress))
	require.NoError(t, err)
	defer r.Body.Close() //nolint:errcheck
	require.Equal(t, http.StatusNotFound, r.StatusCode)
	require.Equal(t, 12, len(r.Header.Get("X-Request-Id")))

	r, err = http.Get(fmt.Sprintf("https://%s/doesnotexist", gatewayAddress))
	if err == nil {
		defer r.Body.Close() //nolint:errcheck
	}
	require.NotNil(t, err)
	require.Contains(t, err.Error(), "failed to verify certificate")

	require.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGINT))
}
