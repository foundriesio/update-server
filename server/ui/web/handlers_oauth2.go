// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/labstack/echo/v4"

	"github.com/foundriesio/update-server/server/ui/api"
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

// deviceAuthError shows the API's status and message, such as an expired code.
func (h handlers) deviceAuthError(c echo.Context, err error) error {
	var apiErr *apiStatusError
	if errors.As(err, &apiErr) {
		return h.handleError(c, apiErr.Status, errors.New(apiErr.Message()))
	}
	return h.handleUnexpected(c, err)
}

func (h handlers) authDeviceConfirm(c echo.Context) error {
	userCode := c.QueryParam("user_code")
	if userCode == "" {
		return h.handleError(c, http.StatusBadRequest, errors.New("user_code is required"))
	}

	var info api.DeviceAuthInfo
	if err := getJson(c.Request().Context(), "/v1/oauth2/device/"+url.PathEscape(userCode), &info); err != nil {
		return h.deviceAuthError(c, err)
	}

	data := struct {
		baseCtx
		UserCode     string
		Scopes       string
		TokenExpires int64
	}{
		baseCtx:      h.baseCtx(c, "API Activation", "settings"),
		UserCode:     info.UserCode,
		Scopes:       info.Scopes,
		TokenExpires: info.TokenExpires,
	}

	return c.Render(http.StatusOK, "device_auth_confirm.html", data)
}

func (h handlers) authDeviceAuthorize(c echo.Context) error {
	req := api.DeviceApproveRequest{
		UserCode:    c.FormValue("user_code"),
		Description: c.FormValue("token_description"),
	}
	if err := postJson(c.Request().Context(), "/v1/oauth2/device/approve", req, nil); err != nil {
		return h.deviceAuthError(c, err)
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
	req := api.DeviceDenyRequest{UserCode: c.FormValue("user_code")}
	if err := postJson(c.Request().Context(), "/v1/oauth2/device/deny", req, nil); err != nil {
		return h.deviceAuthError(c, err)
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
