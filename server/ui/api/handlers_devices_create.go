// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"maps"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

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

func (ca DeviceCa) SignCsr(csr *x509.CertificateRequest) ([]byte, error) {
	if !slices.Equal(ca.CaCert.Subject.OrganizationalUnit, csr.Subject.OrganizationalUnit) {
		return nil, fmt.Errorf("CSR OU %v does not match CA OU %v", csr.Subject.OrganizationalUnit, ca.CaCert.Subject.OrganizationalUnit)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}
	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               csr.Subject,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(7300 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, ca.CaCert, csr.PublicKey, ca.CaKey)
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certBytes}), nil
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

	cert, _, err := genCert(req.Uuid, req.Csr, h.ca)
	if err != nil {
		return EchoError(c, err, http.StatusBadRequest, err.Error())
	}

	sotaBytes := genSotaToml(req, h.ca.DgUrl)

	resp := DeviceCreateResponse{
		RootCrt:   h.ca.RootCert,
		ClientPem: string(cert),
		SotaToml:  string(sotaBytes),
	}
	return c.JSON(http.StatusCreated, resp)
}

func genCert(uuid, csr string, ca *DeviceCa) ([]byte, *x509.CertificateRequest, error) {
	block, _ := pem.Decode([]byte(csr))
	if block == nil {
		return nil, nil, fmt.Errorf("invalid CSR PEM encoding")
	}
	if block.Type != "CERTIFICATE REQUEST" {
		return nil, nil, fmt.Errorf("invalid CSR PEM encoding: %s", block.Type)
	}
	req, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid CSR: %w", err)
	}
	if err := req.CheckSignature(); err != nil {
		return nil, nil, fmt.Errorf("CSR signature verification failed: %w", err)
	}

	if uuid != req.Subject.CommonName {
		return nil, nil, fmt.Errorf("CSR CommonName must match the requested UUID: %s != %s", req.Subject.CommonName, uuid)
	}

	cert, err := ca.SignCsr(req)
	return cert, req, err
}

func genSotaToml(req DeviceCreateRequest, urlBase string) []byte {
	if len(req.SotaConfigDir) == 0 {
		req.SotaConfigDir = "/var/sota"
	}

	sota := SotaToml{
		"tls": {
			"server":      urlBase,
			"ca_source":   "file",
			"pkey_source": "file",
			"cert_source": "file",
		},
		"provision": {
			"server":                  urlBase,
			"primary_ecu_hardware_id": req.HardwareId,
		},
		"uptane": {
			"repo_server": urlBase + "/repo",
			"key_source":  "file",
		},
		"pacman": {
			"type":               "ostree",
			"compose_apps_proxy": urlBase + "/app-proxy-url",
			"ostree_server":      urlBase + "/ostree",
			"packages_file":      "/usr/package.manifest",
		},
		"storage": {
			"type": "sqlite",
			"path": req.SotaConfigDir,
		},
		"import": {
			"tls_cacert_path":     filepath.Join(req.SotaConfigDir, "root.crt"),
			"tls_pkey_path":       filepath.Join(req.SotaConfigDir, "pkey.pem"),
			"tls_clientcert_path": filepath.Join(req.SotaConfigDir, "client.pem"),
		},
	}

	// merge overrides
	for section, kv := range req.Overrides {
		if _, ok := sota[section]; !ok {
			sota[section] = map[string]string{}
		}
		maps.Copy(sota[section], kv)
	}

	var buf bytes.Buffer
	for section, kv := range sota {
		fmt.Fprintf(&buf, "[%s]\n", section)
		for k, v := range kv {
			if len(v) == 0 {
				fmt.Fprintf(&buf, "# %s disabled\n", k)
			} else if v[0] == '"' {
				fmt.Fprintf(&buf, "%s = %s\n", k, v)
			} else {
				fmt.Fprintf(&buf, "%s = \"%s\"\n", k, v)
			}
		}
		fmt.Fprintln(&buf)
	}
	return buf.Bytes()
}
