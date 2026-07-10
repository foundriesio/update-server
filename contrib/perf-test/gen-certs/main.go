// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

// gen-certs generates a CA, a server TLS cert, and N device leaf certs for
// the dg-sat perf test. All crypto happens in-process (no openssl forks) so
// 5000 devices complete in seconds.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"
)

func main() {
	datadir := flag.String("datadir", "", "output root directory (required)")
	numDevices := flag.Int("num-devices", 5000, "number of device certs to generate")
	hostname := flag.String("hostname", "dg-sat", "gateway hostname (server cert SAN)")
	factory := flag.String("factory", "dg-satellite-fake", "device cert OU / factory name")
	flag.Parse()

	if *datadir == "" {
		fmt.Fprintln(os.Stderr, "error: --datadir is required")
		os.Exit(1)
	}

	start := time.Now()

	certsDir := filepath.Join(*datadir, "certs")
	if err := os.MkdirAll(certsDir, 0o755); err != nil {
		fatal("mkdir certs:", err)
	}
	devicesDir := filepath.Join(*datadir, "fake-devices")
	if err := os.MkdirAll(devicesDir, 0o755); err != nil {
		fatal("mkdir fake-devices:", err)
	}

	// --- Factory CA ---
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		fatal("generate CA key:", err)
	}
	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Factory-CA"},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.Add(20 * 365 * 24 * time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		fatal("create CA cert:", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		fatal("parse CA cert:", err)
	}

	writePEM(filepath.Join(certsDir, "factory_ca.pem"), "CERTIFICATE", caDER)
	writeKeyPEM(filepath.Join(certsDir, "factory_ca.key"), caKey)
	writePEM(filepath.Join(certsDir, "cas.pem"), "CERTIFICATE", caDER)
	writePEM(filepath.Join(certsDir, "root.crt"), "CERTIFICATE", caDER)

	// --- Server TLS cert ---
	srvKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		fatal("generate server key:", err)
	}
	srvTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: *hostname},
		DNSNames:     []string{*hostname},
		NotBefore:    now.Add(-5 * time.Minute),
		NotAfter:     now.Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	srvDER, err := x509.CreateCertificate(rand.Reader, srvTemplate, caCert, &srvKey.PublicKey, caKey)
	if err != nil {
		fatal("create server cert:", err)
	}
	writePEM(filepath.Join(certsDir, "tls.pem"), "CERTIFICATE", srvDER)
	writeKeyPEM(filepath.Join(certsDir, "tls.key"), srvKey)

	// --- Device leaf certs (concurrent) ---
	type job struct{ n int }
	jobs := make(chan job, *numDevices)
	for i := 1; i <= *numDevices; i++ {
		jobs <- job{i}
	}
	close(jobs)

	workers := runtime.NumCPU()
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for j := range jobs {
				genDevice(j.n, devicesDir, caCert, caKey, *factory, now)
			}
		}()
	}
	wg.Wait()

	fmt.Printf("generated %d devices in %s\n", *numDevices, time.Since(start).Round(time.Millisecond))
}

func genDevice(n int, devicesDir string, caCert *x509.Certificate, caKey *ecdsa.PrivateKey, factory string, now time.Time) {
	devKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		fatal(fmt.Sprintf("device-%d key:", n), err)
	}
	id := uuid.New().String()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(int64(n) + 2), // +2: 1=CA, 2=server
		Subject:      pkix.Name{CommonName: id, OrganizationalUnit: []string{factory}},
		NotBefore:    now.Add(-5 * time.Minute),
		NotAfter:     now.Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	devDER, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &devKey.PublicKey, caKey)
	if err != nil {
		fatal(fmt.Sprintf("device-%d cert:", n), err)
	}

	dir := filepath.Join(devicesDir, fmt.Sprintf("device-%d", n))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fatal("mkdir device:", err)
	}

	// cert + key in one file (requests/urllib3 accepts combined PEM for client.cert)
	f, err := os.Create(filepath.Join(dir, "client.pem"))
	if err != nil {
		fatal("create client.pem:", err)
	}
	defer f.Close()
	if err := pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: devDER}); err != nil {
		fatal("encode device cert:", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(devKey)
	if err != nil {
		fatal("marshal device key:", err)
	}
	if err := pem.Encode(f, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		fatal("encode device key:", err)
	}
}

func writePEM(path, blockType string, der []byte) {
	f, err := os.Create(path)
	if err != nil {
		fatal("create "+path+":", err)
	}
	defer f.Close()
	if err := pem.Encode(f, &pem.Block{Type: blockType, Bytes: der}); err != nil {
		fatal("write "+path+":", err)
	}
}

func writeKeyPEM(path string, key *ecdsa.PrivateKey) {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		fatal("marshal key:", err)
	}
	writePEM(path, "EC PRIVATE KEY", der)
}

func fatal(msg string, err error) {
	fmt.Fprintf(os.Stderr, "gen-certs: %s %v\n", msg, err)
	os.Exit(1)
}
