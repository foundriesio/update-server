// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

type pkiCertSummary struct {
	Serial  string
	Issuer  string
	Subject string
	Expires string
}

type pkiSection struct {
	Title string
	Certs []pkiCertSummary
	Raw   string
}

func (h handlers) pki(c echo.Context) error {
	ctx := c.Request().Context()
	sections := []struct {
		title    string
		resource string
	}{
		{"Root CA", "root.crt"},
		{"Gateway TLS Certificate", "tls.crt"},
		{"Device CA", "device-ca.crt"},
	}

	var certs map[string]string
	if err := getJson(ctx, "/v1/pki/cert?name=root.crt&name=tls.crt&name=device-ca.crt", &certs); err != nil {
		return h.handleUnexpected(c, err)
	}

	result := make([]pkiSection, 0, len(sections)+1)
	for _, s := range sections {
		certSummaries, err := parseCertSummaries([]byte(certs[s.resource]))
		if err != nil {
			return h.handleUnexpected(c, err)
		}
		result = append(result, pkiSection{Title: s.title, Certs: certSummaries, Raw: certs[s.resource]})
	}

	caBytes, err := getRaw(ctx, "/v1/pki/cas.pem")
	if err != nil {
		return h.handleUnexpected(c, err)
	}
	certSummaries, err := parseCertSummaries(caBytes)
	if err != nil {
		return h.handleUnexpected(c, err)
	}
	result = append(result, pkiSection{Title: "CA Bundles", Certs: certSummaries, Raw: string(caBytes)})

	pageCtx := struct {
		baseCtx
		Sections []pkiSection
	}{
		baseCtx:  h.baseCtx(c, "PKI", "pki"),
		Sections: result,
	}
	return c.Render(http.StatusOK, "pki.html", pageCtx)
}

// parseCertSummaries mirrors cli/subcommands/pki/show.go's printCertSummaries:
// a bundle such as the CA bundle can contain more than one certificate.
func parseCertSummaries(pemBytes []byte) ([]pkiCertSummary, error) {
	var out []pkiCertSummary
	rest := pemBytes
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		out = append(out, pkiCertSummary{
			Serial:  formatSerial(cert.SerialNumber.Bytes()),
			Issuer:  cert.Issuer.String(),
			Subject: cert.Subject.String(),
			Expires: cert.NotAfter.Format(time.RFC3339),
		})
	}
	return out, nil
}

func formatSerial(b []byte) string {
	if len(b) == 0 {
		return "0"
	}
	parts := make([]string, len(b))
	for i, by := range b {
		parts[i] = fmt.Sprintf("%02x", by)
	}
	return strings.Join(parts, ":")
}
