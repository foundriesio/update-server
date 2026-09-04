// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"errors"
	"net/http"
	"os"
	"slices"

	"github.com/labstack/echo/v4"

	"github.com/foundriesio/update-server/storage"
)

type CertResp map[string]string

type CertOpts struct {
	Names []string `query:"name"`
}

// @Summary Query for one or more server certificates.
// @Param _ query CertOpts false "Certificate to retrieve. Valid options are root.crt, tls.crt, device-ca.crt"
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

	allowed := []string{"root.crt", "tls.crt", "device-ca.crt"}
	resp := make(CertResp, len(opts.Names))

	for _, name := range opts.Names {
		if !slices.Contains(allowed, name) {
			msg := "invalid certificate requested: " + name
			return EchoError(c, errors.New(msg), http.StatusBadRequest, msg)
		}
		buf, err := h.fs.Certs.ReadFile(name)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				buf = nil
			} else {
				return EchoError(c, err, http.StatusInternalServerError, "Failed to read certificate: "+name)
			}
		}
		resp[name] = string(buf)
	}
	return c.JSON(http.StatusOK, resp)
}

// @Summary Get the CA bundle trusted for device mTLS
// @Produce text/plain
// @Success 200
// @Router  /pki/cas.pem [get]
func (h handlers) pkiCasPem(c echo.Context) error {
	buf, err := h.fs.Certs.ReadFile(storage.CertsCasPemFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return EchoError(c, err, http.StatusNotFound, "Not found")
		}
		return EchoError(c, err, http.StatusInternalServerError, "Failed to read certificate")
	}
	return c.String(http.StatusOK, string(buf))
}
