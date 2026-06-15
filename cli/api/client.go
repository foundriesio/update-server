// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"log/slog"
	"net/http"

	"github.com/foundriesio/update-server/cli/config"
)

type Api struct {
	URL string

	Client *http.Client
}

func NewClient(appCtx config.Context) *Api {
	return &Api{
		URL: appCtx.URL,

		Client: &http.Client{
			Transport: &authTransport{
				Token:     appCtx.Token,
				Transport: http.DefaultTransport,
			},
		},
	}
}

type authTransport struct {
	Token     string
	Transport http.RoundTripper
}

// RoundTrip implements the http.RoundTripper interface in a way which adds
// the Authorization header to each request.
func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	reqBodyClosed := false
	if req.Body != nil {
		defer func() {
			if !reqBodyClosed {
				if err := req.Body.Close(); err != nil {
					slog.Error("failed to close request body", "error", err)
				}
			}
		}()
	}

	req2 := req.Clone(req.Context())
	req2.Header.Set("Authorization", "Bearer "+t.Token)

	// req.Body is assumed to be closed by the base RoundTripper.
	reqBodyClosed = true
	return t.Transport.RoundTrip(req2)
}
