// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

type pkiCertificateRecord struct {
	Title       string
	Description string
	Subject     string
	Issuer      string
	Expires     string
	Serial      string
	Raw         string
	ExpiresAt   time.Time
	Configured  bool
}

type pkiRegistrySummary struct {
	TrustItems     int
	Configured     int
	NotConfigured  int
	EarliestExpiry string
}

type pkiRegistryData struct {
	Summary pkiRegistrySummary
	Records []pkiCertificateRecord
}

type pkiPageCtx struct {
	baseCtx
	Summary pkiRegistrySummary
	Records []pkiCertificateRecord
}

type pkiCertificateSource struct {
	title       string
	description string
	resource    string
}

var pkiCertificateSources = []pkiCertificateSource{
	{title: "Root CA", description: "Server trust anchor", resource: "root.crt"},
	{title: "Gateway TLS Certificate", description: "Gateway server identity", resource: "tls.crt"},
	{title: "Device CA", description: "Device identity authority", resource: "device-ca.crt"},
	{title: "CA Bundle", description: "Trusted certificate bundle", resource: "cas.pem"},
}

func (h handlers) pki(c echo.Context) error {
	ctx := c.Request().Context()
	var certs map[string]string
	if err := getJson(ctx, "/v1/pki/cert?name=root.crt&name=tls.crt&name=device-ca.crt&name=cas.pem", &certs); err != nil {
		return h.handleUnexpected(c, err)
	}

	registry, err := buildPkiRegistryData(certs)
	if err != nil {
		return h.handleUnexpected(c, err)
	}

	pageCtx := pkiPageCtx{
		baseCtx: h.baseCtx(c, "PKI", "pki"),
		Summary: registry.Summary,
		Records: registry.Records,
	}
	return c.Render(http.StatusOK, "pki.html", pageCtx)
}

func buildPkiRegistryData(certs map[string]string) (pkiRegistryData, error) {
	data := pkiRegistryData{
		Records: make([]pkiCertificateRecord, 0, len(pkiCertificateSources)),
	}
	var earliest time.Time

	for _, source := range pkiCertificateSources {
		records, err := parsePkiCertificates([]byte(certs[source.resource]))
		if err != nil {
			return pkiRegistryData{}, fmt.Errorf("parse %s: %w", source.resource, err)
		}
		if len(records) == 0 {
			data.Records = append(data.Records, pkiCertificateRecord{
				Title:       source.title,
				Description: source.description,
			})
			continue
		}

		for index := range records {
			record := &records[index]
			record.Title = source.title
			if len(records) > 1 {
				record.Title = fmt.Sprintf("%s %d of %d", source.title, index+1, len(records))
			}
			record.Description = source.description
			data.Records = append(data.Records, *record)
			if earliest.IsZero() || record.ExpiresAt.Before(earliest) {
				earliest = record.ExpiresAt
			}
		}
	}

	data.Summary.TrustItems = len(data.Records)
	for _, record := range data.Records {
		if record.Configured {
			data.Summary.Configured++
		} else {
			data.Summary.NotConfigured++
		}
	}
	if earliest.IsZero() {
		data.Summary.EarliestExpiry = "—"
	} else {
		data.Summary.EarliestExpiry = earliest.Format(time.RFC3339)
	}
	return data, nil
}

func parsePkiCertificates(pemBytes []byte) ([]pkiCertificateRecord, error) {
	var records []pkiCertificateRecord
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
		records = append(records, pkiCertificateRecord{
			Subject:    cert.Subject.String(),
			Issuer:     cert.Issuer.String(),
			Expires:    cert.NotAfter.Format(time.RFC3339),
			Serial:     formatSerial(cert.SerialNumber.Bytes()),
			Raw:        string(pem.EncodeToMemory(block)),
			ExpiresAt:  cert.NotAfter,
			Configured: true,
		})
	}
	if len(records) == 0 && len(bytes.TrimSpace(pemBytes)) != 0 {
		return nil, fmt.Errorf("no PEM CERTIFICATE block found")
	}
	return records, nil
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
