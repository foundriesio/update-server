// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package pki

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/foundriesio/update-server/cli/api"
)

var (
	justRoot     bool
	justCas      bool
	justTls      bool
	justDeviceCa bool
	raw          bool
)

var showCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the server's PKI certificates",
	Long: `Show the server's PKI certificates: root CA, CA bundle, gateway TLS
certificate, and device signing CA (if configured).

By default, all certificates are shown, each preceded by a header, with a
summary of each certificate's serial number, issuer, subject, and expiration.
Use --raw to print the raw PEM content instead, suitable for redirecting to
a file. Use one of the --just-* flags to print a single certificate.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		pki := api.CtxGetApi(cmd.Context()).Pki()
		certs := []string{"root.crt", "tls.crt", "device-ca.crt"}
		cas := true
		switch {
		case justRoot:
			certs = []string{"root.crt"}
			cas = false
		case justTls:
			certs = []string{"tls.crt"}
			cas = false
		case justDeviceCa:
			certs = []string{"device-ca.crt"}
			cas = false
		case justCas:
			certs = []string{}
		}
		doShow(pki, certs, cas)
	},
}

func init() {
	PkiCmd.AddCommand(showCmd)
	showCmd.Flags().BoolVar(&justRoot, "just-root", false, "Print only the root CA certificate")
	showCmd.Flags().BoolVar(&justCas, "just-cas", false, "Print only the CA bundle trusted for device mTLS")
	showCmd.Flags().BoolVar(&justTls, "just-tls", false, "Print only the gateway TLS certificate")
	showCmd.Flags().BoolVar(&justDeviceCa, "just-device-ca", false, "Print only the device signing CA certificate")
	showCmd.Flags().BoolVar(&raw, "raw", false, "Print the raw PEM content instead of a summary")
	showCmd.MarkFlagsMutuallyExclusive("just-root", "just-cas", "just-tls", "just-device-ca")
}

func doShow(pki api.PkiApi, certs []string, cas bool) {
	if len(certs) > 0 {
		resp, err := pki.GetCerts(certs)
		cobra.CheckErr(err)

		var certTitles = map[string]string{
			"root.crt":      "Root CA",
			"tls.crt":       "Gateway TLS Certificate",
			"device-ca.crt": "Device CA",
		}

		for _, name := range certs {
			fmt.Printf("%s:\n", certTitles[name])
			val := resp[name]
			if len(val) == 0 {
				fmt.Println("not configured")
				fmt.Println()
			} else if raw {
				fmt.Println(val)
			} else {
				cobra.CheckErr(printCertSummaries([]byte(val)))
			}
		}
	}
	if cas {
		fmt.Println("CA Bundles:")
		certBytes, err := pki.CasPem()
		cobra.CheckErr(err)
		if len(certBytes) > 0 {
			if raw {
				fmt.Println(string(certBytes))
			} else {
				cobra.CheckErr(printCertSummaries(certBytes))
			}
		}
	}
}

// printCertSummaries pretty-prints every certificate found in pemBytes. A
// bundle such as the CA bundle can contain more than one certificate.
func printCertSummaries(pemBytes []byte) error {
	rest := pemBytes
	found := 0
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
			return err
		}
		printCertSummary(cert)
		found++
	}
	if found == 0 {
		return fmt.Errorf("no certificates found in PEM data")
	}
	return nil
}

func printCertSummary(cert *x509.Certificate) {
	fmt.Printf("  Serial:  %s\n", formatSerial(cert.SerialNumber.Bytes()))
	fmt.Printf("  Issuer:  %s\n", cert.Issuer)
	fmt.Printf("  Subject: %s\n", cert.Subject)
	fmt.Printf("  Expires: %s\n", cert.NotAfter.Format("2006-01-02 15:04:05 MST"))
	fmt.Println()
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
