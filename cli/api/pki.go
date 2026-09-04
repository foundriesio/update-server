// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"encoding/json"
	"io"
	"strings"
)

type PkiApi struct {
	api *Api
}

func (a *Api) Pki() PkiApi {
	return PkiApi{api: a}
}

func (p PkiApi) get(resource string) ([]byte, error) {
	resp, err := p.api.get(resource)
	if err != nil {
		return nil, err
	}
	defer p.api.closeHttpBody(resp.Body)
	return io.ReadAll(resp.Body)
}

func (p PkiApi) GetCerts(certs []string) (map[string]string, error) {
	var query strings.Builder
	query.WriteString("?")
	for idx, cert := range certs {
		query.WriteString("name=")
		query.WriteString(cert)
		if idx < len(certs)-1 {
			query.WriteString("&")
		}
	}
	buf, err := p.get("/v1/pki/cert" + query.String())
	if err != nil {
		return nil, err
	}
	var resp map[string]string
	if err := json.Unmarshal(buf, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (p PkiApi) CasPem() ([]byte, error) {
	return p.get("/v1/pki/cas.pem")
}
