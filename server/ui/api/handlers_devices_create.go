// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/foundriesio/update-server/storage"
	"github.com/labstack/echo/v4"
)

type DeviceCa struct {
	CaCert   *x509.Certificate
	CaKey    crypto.Signer
	RootCert string
	DgUrl    string
}

func LoadDeviceCa(fs *storage.FsHandle) (*DeviceCa, error) {
	buf, err := fs.Certs.ReadFile(storage.CertsDeviceCaKeyFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	first, rest := pem.Decode(buf)
	if first == nil || len(strings.TrimSpace(string(rest))) > 0 {
		return nil, fmt.Errorf("malformed PEM data for %s", storage.CertsDeviceCaKeyFile)
	}

	key, err := x509.ParseECPrivateKey(first.Bytes)
	if err != nil {
		return nil, err
	}

	buf, err = fs.Certs.ReadFile(storage.CertsDeviceCaPemFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	first, rest = pem.Decode(buf)
	if first == nil || len(strings.TrimSpace(string(rest))) > 0 {
		return nil, fmt.Errorf("malformed PEM data for %s", storage.CertsDeviceCaPemFile)
	}

	cert, err := x509.ParseCertificate(first.Bytes)
	if err != nil {
		return nil, err
	}

	if !cert.IsCA {
		return nil, fmt.Errorf("certificate in %s is not a CA certificate", storage.CertsDeviceCaPemFile)
	}

	buf, err = fs.Certs.ReadFile(storage.CertsRootPemFile)
	if err != nil {
		return nil, err
	}
	rootCrt := string(buf)

	// load dg tls cert to get the DG URL
	buf, err = fs.Certs.ReadFile(storage.CertsTlsPemFile)
	if err != nil {
		return nil, err
	}
	first, rest = pem.Decode(buf)
	if first == nil || len(strings.TrimSpace(string(rest))) > 0 {
		return nil, fmt.Errorf("malformed PEM data for %s", storage.CertsTlsPemFile)
	}

	tlsCert, err := x509.ParseCertificate(first.Bytes)
	if err != nil {
		return nil, err
	}

	return &DeviceCa{
		CaCert:   cert,
		CaKey:    key,
		RootCert: rootCrt,
		DgUrl:    "https://" + tlsCert.DNSNames[0] + ":8443",
	}, nil
}

type SotaToml map[string]map[string]string

type DeviceCreateRequest struct {
	Uuid          string   `json:"uuid"`
	Name          string   `json:"name"` // todo validate pattern and length
	Group         string   `json:"group"`
	HardwareId    string   `json:"hardware-id"`
	Csr           string   `json:"csr"`
	SotaConfigDir string   `json:"sota-config-dir"`
	Overrides     SotaToml `json:"overrides"`
}

type DeviceCreateResponse struct {
	RootCrt   string `json:"root.crt"`
	ClientPem string `json:"client.pem"`
	SotaToml  string `json:"sota.toml"`
}

// @Summary Create device. Used by lmp-device-register compliant tooling
// @Accept  json
// @Produce json
// @Param data body DeviceCreateRequest true "Device creation request"
// @Success 201 {object} DeviceCreateResponse
// @Router  /devices/ [post]
func (h handlers) deviceCreate(c echo.Context) error {
	var req DeviceCreateRequest
	if err := c.Bind(&req); err != nil {
		return EchoError(c, err, http.StatusBadRequest, "Bad JSON body")
	}

	if len(req.Uuid) == 0 {
		return c.JSON(http.StatusBadRequest, "Uuid is required")
	}

	return nil
}
