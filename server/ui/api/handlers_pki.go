// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
)

type CertResp map[string]string

type CertOpts struct {
	Names []string `query:"name"`
}

// @Summary Query for one or more server certificates.
// @Param _ query CertOpts false "Certificate to retrieve. Valid options are root.crt, tls.crt, device-ca.crt, cas.pem"
// @Produce json
// @Success 200 {object} CertResp
// @Router  /pki/cert [get]
func (h handlers) pkiCert(c echo.Context) error {
	var opts CertOpts
	if err := c.Bind(&opts); err != nil {
		return EchoError(c, err, http.StatusBadRequest, "Failed to parse query options")
	}
	if len(opts.Names) == 0 {
		msg := "no certificate names specified"
		return EchoError(c, errors.New(msg), http.StatusBadRequest, msg)
	}

	resp := make(CertResp, len(opts.Names))
	for _, name := range opts.Names {
		buf, err := h.storage.ReadCert(name)
		if err != nil {
			return EchoError(c, err, http.StatusInternalServerError, "Failed to read certificate: "+name)
		}
		resp[name] = buf
	}
	return c.JSON(http.StatusOK, resp)
}
