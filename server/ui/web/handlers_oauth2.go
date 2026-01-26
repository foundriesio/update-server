// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/foundriesio/update-server/storage/users"
)

func (h handlers) authDevice(c echo.Context) error {
	userCode := c.QueryParam("user_code")

	data := struct {
		baseCtx
		UserCode string
	}{
		baseCtx:  h.baseCtx(c, "API Activation", "settings"),
		UserCode: userCode,
	}

	return c.Render(http.StatusOK, "device_auth.html", data)
}

var errDeviceAuthInvalidCode = errors.New("invalid user code")
var errDeviceAuthExpired = errors.New("authorization code expired")
var errDeviceAuthAlreadyHandled = errors.New("this device has already been authorized or denied")

func (h handlers) authDeviceConfirm(c echo.Context) error {
	userCode := c.QueryParam("user_code")
	if userCode == "" {
		return EchoError(c, nil, http.StatusBadRequest, "user_code is required")
	}

	auth, err := h.users.GetDeviceAuthByUserCode(userCode)
	if err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to get device authorization")
	}
	if auth == nil {
		return EchoError(c, errDeviceAuthInvalidCode, http.StatusNotFound, errDeviceAuthInvalidCode.Error())
	}

	if auth.ExpiresAt < time.Now().Unix() {
		return EchoError(c, errDeviceAuthExpired, http.StatusBadRequest, errDeviceAuthExpired.Error())
	}
	if auth.Authorized || auth.Denied {
		return EchoError(c, errDeviceAuthAlreadyHandled, http.StatusBadRequest, errDeviceAuthAlreadyHandled.Error())
	}

	// Get the current user from session to intersect scopes
	session := CtxGetSession(c.Request().Context())
	if session == nil || session.User == nil {
		return EchoError(c, nil, http.StatusUnauthorized, "Not authenticated")
	}

	scopes, err := users.ScopesFromString(auth.Scopes)
	if err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to parse requested scopes: "+auth.Scopes)
	}
	allowedScopes := scopes & session.User.AllowedScopes

	data := struct {
		baseCtx
		UserCode     string
		Scopes       string
		TokenExpires int64
	}{
		baseCtx:      h.baseCtx(c, "API Activation", "settings"),
		UserCode:     userCode,
		Scopes:       allowedScopes.String(),
		TokenExpires: auth.TokenExpires,
	}

	return c.Render(http.StatusOK, "device_auth_confirm.html", data)
}

func (h handlers) authDeviceAuthorize(c echo.Context) error {
	userCode := c.FormValue("user_code")
	if userCode == "" {
		return EchoError(c, nil, http.StatusBadRequest, "user_code is required")
	}
	description := c.FormValue("token_description")

	auth, err := h.users.GetDeviceAuthByUserCode(userCode)
	if err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to get device authorization")
	}
	if auth == nil {
		return EchoError(c, errDeviceAuthInvalidCode, http.StatusNotFound, errDeviceAuthInvalidCode.Error())
	}

	if auth.ExpiresAt < time.Now().Unix() {
		return EchoError(c, errDeviceAuthExpired, http.StatusBadRequest, errDeviceAuthExpired.Error())
	}

	if auth.Authorized || auth.Denied {
		return EchoError(c, errDeviceAuthAlreadyHandled, http.StatusBadRequest, errDeviceAuthAlreadyHandled.Error())
	}

	session := CtxGetSession(c.Request().Context())

	scopes, err := users.ScopesFromString(auth.Scopes)
	if err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to parse requested scopes: "+auth.Scopes)
	}
	allowedScopes := scopes & session.User.AllowedScopes

	if err := session.User.ApproveAuthorization(auth.DeviceCode, description, allowedScopes); err != nil {
		return h.handleUnexpected(c, err)
	}

	data := struct {
		baseCtx
		Message string
	}{
		baseCtx: h.baseCtx(c, "API Activation", "settings"),
		Message: "Activation successful! You can close this window and return to your device.",
	}
	return c.Render(http.StatusOK, "device_auth_success.html", data)
}

// authDeviceDeny handles the user denying the device authorization
// POST /auth/device/deny
func (h handlers) authDeviceDeny(c echo.Context) error {
	userCode := c.FormValue("user_code")
	if userCode == "" {
		return EchoError(c, nil, http.StatusBadRequest, "user_code is required")
	}

	auth, err := h.users.GetDeviceAuthByUserCode(userCode)
	if err != nil {
		return h.handleError(c, http.StatusInternalServerError, errors.New("failed to get device authorization"))
	}
	if auth == nil {
		return h.handleError(c, http.StatusNotFound, errDeviceAuthInvalidCode)
	}

	if auth.ExpiresAt < time.Now().Unix() {
		return h.handleError(c, http.StatusBadRequest, errDeviceAuthExpired)
	}

	if auth.Authorized || auth.Denied {
		return h.handleError(c, http.StatusBadRequest, errDeviceAuthAlreadyHandled)
	}

	session := CtxGetSession(c.Request().Context())
	if err := session.User.DenyDeviceAuth(auth.DeviceCode); err != nil {
		return h.handleUnexpected(c, err)
	}

	data := struct {
		baseCtx
		Message string
	}{
		baseCtx: h.baseCtx(c, "API Activation", "settings"),
		Message: "Activation denied. You can close this window.",
	}
	return c.Render(http.StatusOK, "device_auth_success.html", data)
}
