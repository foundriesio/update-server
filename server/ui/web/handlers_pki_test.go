// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/foundriesio/update-server/server/ui/web/templates"
)

func TestBuildPkiRegistryDataBuildsConfiguredAndPlaceholderRecords(t *testing.T) {
	data, err := buildPkiRegistryData(map[string]string{
		"root.crt": testCertificatePEM(t, "root", 1, time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)),
		"tls.crt":  testCertificatePEM(t, "gateway", 2, time.Date(2031, 1, 2, 3, 4, 5, 0, time.UTC)),
	})
	if err != nil {
		t.Fatalf("buildPkiRegistryData() error = %v", err)
	}

	wantSummary := pkiRegistrySummary{
		TrustItems:     4,
		Configured:     2,
		NotConfigured:  2,
		EarliestExpiry: "2030-01-02T03:04:05Z",
	}
	if data.Summary != wantSummary {
		t.Errorf("Summary = %#v, want %#v", data.Summary, wantSummary)
	}
	if len(data.Records) != 4 {
		t.Fatalf("len(Records) = %d, want 4", len(data.Records))
	}
	if record := data.Records[0]; record.Title != "Root CA" || !record.Configured {
		t.Errorf("Records[0] = %#v, want configured Root CA", record)
	}
	if record := data.Records[2]; record.Title != "Device CA" || record.Configured {
		t.Errorf("Records[2] = %#v, want unconfigured Device CA", record)
	}
}

func TestBuildPkiRegistryDataFlattensBundle(t *testing.T) {
	bundle := testCertificatePEM(t, "bundle-one", 11, time.Date(2032, 1, 2, 3, 4, 5, 0, time.UTC)) +
		testCertificatePEM(t, "bundle-two", 12, time.Date(2033, 1, 2, 3, 4, 5, 0, time.UTC))

	data, err := buildPkiRegistryData(map[string]string{"cas.pem": bundle})
	if err != nil {
		t.Fatalf("buildPkiRegistryData() error = %v", err)
	}
	if len(data.Records) != 5 {
		t.Fatalf("len(Records) = %d, want 5", len(data.Records))
	}
	if got := data.Records[3].Title; got != "CA Bundle 1 of 2" {
		t.Errorf("Records[3].Title = %q, want %q", got, "CA Bundle 1 of 2")
	}
	if got := data.Records[4].Title; got != "CA Bundle 2 of 2" {
		t.Errorf("Records[4].Title = %q, want %q", got, "CA Bundle 2 of 2")
	}
	assertRawCertificateSerial(t, data.Records[3].Raw, 11)
	assertRawCertificateSerial(t, data.Records[4].Raw, 12)
}

func TestBuildPkiRegistryDataRejectsNonEmptyPayloadWithoutCertificate(t *testing.T) {
	_, err := buildPkiRegistryData(map[string]string{"root.crt": "not a PEM certificate"})
	if err == nil {
		t.Fatal("buildPkiRegistryData() error = nil, want malformed root certificate error")
	}
	if !strings.Contains(err.Error(), "root.crt") {
		t.Errorf("buildPkiRegistryData() error = %q, want resource name", err)
	}
}

func TestBuildPkiRegistryDataCreatesPlaceholderForAbsentResource(t *testing.T) {
	data, err := buildPkiRegistryData(map[string]string{})
	if err != nil {
		t.Fatalf("buildPkiRegistryData() error = %v", err)
	}

	wantSummary := pkiRegistrySummary{
		TrustItems:     4,
		Configured:     0,
		NotConfigured:  4,
		EarliestExpiry: "—",
	}
	if data.Summary != wantSummary {
		t.Errorf("Summary = %#v, want %#v", data.Summary, wantSummary)
	}
	for i, record := range data.Records {
		if record.Configured {
			t.Errorf("Records[%d].Configured = true, want false", i)
		}
	}
}

func TestPkiTemplateRendersRegistryCards(t *testing.T) {
	const rawCertificate = "-----BEGIN CERTIFICATE-----\ntest certificate\n-----END CERTIFICATE-----\n"
	ctx := pkiPageCtx{
		baseCtx: baseCtx{
			Title:     "PKI",
			BrandName: "Foundries Update Server",
			Version:   "test",
		},
		Summary: pkiRegistrySummary{
			TrustItems:     2,
			Configured:     1,
			NotConfigured:  1,
			EarliestExpiry: "2030-01-02T03:04:05Z",
		},
		Records: []pkiCertificateRecord{
			{
				Title:       "Root CA",
				Description: "Server trust anchor",
				Subject:     "CN=root",
				Issuer:      "CN=root",
				Expires:     "2030-01-02T03:04:05Z",
				Serial:      "01",
				Raw:         rawCertificate,
				Configured:  true,
			},
			{
				Title:       "Device CA",
				Description: "Device identity authority",
			},
		},
	}

	var rendered bytes.Buffer
	if err := templates.Templates.ExecuteTemplate(&rendered, "pki.html", ctx); err != nil {
		t.Fatalf("ExecuteTemplate() error = %v", err)
	}
	page := rendered.String()

	for _, text := range []string{
		"<header class=\"heading-group heading-group--page\">",
		"<h1 class=\"heading heading--page\">PKI</h1>",
		"<h2 class=\"heading heading--section\" id=\"certificate-registry-title\">Certificate Registry</h2>",
		"<ol class=\"pki-registry__records\">",
		"<li class=\"pki-registry__record pki-registry__record--configured\">",
		"<h3 class=\"heading heading--entity\">Root CA</h3>",
		"Trust items",
		"Configured",
		"Not configured",
		"Earliest expiry",
		"Subject not configured",
		"Issuer not configured",
		"Expires not configured",
		"Serial number not configured",
		"RAW CERTIFICATE",
		"Show",
		"Hide",
		rawCertificate,
	} {
		if !strings.Contains(page, text) {
			t.Errorf("rendered page does not contain %q", text)
		}
	}
	if got := strings.Count(page, "<article"); got != 2 {
		t.Errorf("article count = %d, want 2", got)
	}
	for _, label := range []string{"Subject", "Issuer", "Expires", "Serial"} {
		if got := strings.Count(page, "<dt>"+label+"</dt>"); got != 2 {
			t.Errorf("%s fact count = %d, want 2", label, got)
		}
	}
	if got := strings.Count(page, "class=\"pki-registry__raw\""); got != 1 {
		t.Errorf("raw disclosure count = %d, want 1", got)
	}
	for _, forbidden := range []string{
		"Certificate authorities, gateway identity, and trusted bundles for this update server.",
		"Trust infrastructure",
		">Details<",
		"View Certificate Text",
		"View vertificate",
	} {
		if strings.Contains(page, forbidden) {
			t.Errorf("rendered page unexpectedly contains %q", forbidden)
		}
	}
}

func testCertificatePEM(t *testing.T, commonName string, serial int64, notAfter time.Time) string {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func assertRawCertificateSerial(t *testing.T, raw string, want int64) {
	t.Helper()

	block, rest := pem.Decode([]byte(raw))
	if block == nil {
		t.Fatal("raw certificate does not contain a PEM block")
	}
	if len(strings.TrimSpace(string(rest))) != 0 {
		t.Fatalf("raw certificate contains an unexpected second PEM block: %q", rest)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("ParseCertificate() error = %v", err)
	}
	if got := cert.SerialNumber.Int64(); got != want {
		t.Errorf("raw certificate serial = %d, want %d", got, want)
	}
}
