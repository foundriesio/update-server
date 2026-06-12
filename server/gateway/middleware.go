// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package gateway

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
)

func (h handlers) authDevice(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		req := c.Request()
		if len(req.TLS.PeerCertificates) == 0 {
			return c.String(http.StatusForbidden, "no client certificate provided")
		}
		cert := req.TLS.PeerCertificates[0]
		uuid := cert.Subject.CommonName
		ctx := req.Context()
		log := CtxGetLog(ctx).With("device", uuid)
		ctx = CtxWithLog(ctx, log)

		pub, err := pubkey(cert)
		if err != nil {
			return c.String(http.StatusForbidden, fmt.Sprintf("unable to extract device's public key: %s", err))
		}

		device, err := h.storage.DeviceGet(uuid)

		if err != nil {
			log.Error("Unable to query for device", "error", err)
			return c.String(http.StatusBadGateway, err.Error())
		} else if device == nil {
			device, err = h.storage.DeviceCreate(cert.Subject.CommonName, pub)
			if err != nil {
				log.Error("Unable to create device", "error", err)
				return c.String(http.StatusBadGateway, "Unable to create device")
			}
			log.Info("Created device")
		} else if device.Deleted {
			return c.String(http.StatusForbidden, fmt.Sprintf("Device(%s) has been deleted", uuid))
		} else if pub != device.PubKey {
			/*if err := device.RotatePubKey(pub); err != nil {
				return c.String(http.StatusForbidden, err.Error())
			}*/
			return c.String(http.StatusBadGateway, "Key rotation is not supported")
		}

		ctx = CtxWithDevice(ctx, device)
		c.SetRequest(req.WithContext(ctx))

		return next(c)
	}
}

func (h handlers) authToken(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		token := c.Request().URL.Query().Get("token")
		if len(token) == 0 {
			return EchoError(c, errors.New("missing token"), http.StatusUnauthorized, "Missing token")
		}

		uuid, ok := h.tokenCache.Get(token)
		if !ok {
			return c.String(http.StatusUnauthorized, "invalid or expired token")
		}
		ctx := c.Request().Context()
		log := CtxGetLog(ctx).With("device", uuid)
		ctx = CtxWithLog(ctx, log)

		device, err := h.storage.DeviceGet(uuid)
		if err != nil {
			log.Error("Unable to query for device", "error", err)
			return c.String(http.StatusBadGateway, "Unable to look up device")
		} else if device == nil || device.Deleted {
			return c.String(http.StatusUnauthorized, "invalid device")
		}

		ctx = CtxWithDevice(ctx, device)
		c.SetRequest(c.Request().WithContext(ctx))
		return next(c)
	}
}

func (h handlers) checkinDevice(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		req := c.Request()
		ctx := req.Context()
		d := CtxGetDevice(ctx)

		apps := getHeader(req, "x-ats-dockerapps", d.Apps)
		hash := getHeader(req, "x-ats-ostreehash", d.OstreeHash)
		tag := getHeader(req, "x-ats-tags", d.Tag)
		target := getHeader(req, "x-ats-target", d.TargetName)

		if err := d.CheckIn(target, tag, hash, apps); err != nil {
			log := CtxGetLog(ctx)
			log.Error("Failed to update device check-in info", "error", err)
		}
		return next(c)
	}
}

func pubkey(cert *x509.Certificate) (string, error) {
	derBytes, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return "", err
	}
	block := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: derBytes,
	}
	return string(pem.EncodeToMemory(block)), nil
}

func getHeader(req *http.Request, header, defVal string) string {
	// Differentiate between an empty header value (unset) and missing header value (ignore).
	if v := req.Header.Values(header); len(v) > 0 {
		return v[0]
	} else {
		return defVal
	}
}
