// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

//nolint:goconst // There are many repetitive error codes in this module - let them duplicate for now.
package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/foundriesio/update-server/storage/users"
)

type oauth2Handlers struct {
	users *users.Storage
}

type DeviceCodeRequest struct {
	Scopes       string `form:"scope"`
	TokenExpires int64  `form:"token_expires"`
}

type DeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"` // seconds until expiry, per RFC 8628
	Interval                int    `json:"interval"`
}

type DeviceTokenRequest struct {
	DeviceCode string `form:"device_code"`
	GrantType  string `form:"grant_type"`
}

type DeviceTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Expires     int64  `json:"expires"`
	Scopes      string `json:"scope"`
}

type oauth2Error struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// @Summary Initiate OAuth2 Device Authorization
// @Accept json
// @Param data body DeviceCodeRequest true "Device code request"
// @Produce json
// @Success 200
// @Router  /oauth2/device/code [post]
func (h oauth2Handlers) oauth2DeviceCode(c echo.Context) error {
	var req DeviceCodeRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, oauth2Error{
			Error:            "invalid_request",
			ErrorDescription: "Invalid request body",
		})
	}

	scopes := strings.Split(req.Scopes, ",")
	if len(scopes) == 1 {
		// might be lmp-dr
		parts := strings.Split(scopes[0], ":")
		if len(parts) == 3 {
			// looks like lmp-dr, convert to proper scope string
			req.Scopes = parts[1] + ":" + parts[2]
		}
		CtxGetLog(c.Request().Context()).Info("Assuming lmp-device-register format for scopes", "scopes", req.Scopes)

		if req.TokenExpires == 0 {
			// lmp-dr doesn't send token_expires, set to 5 minutes
			req.TokenExpires = time.Now().Add(5 * time.Minute).Unix()
		}
	}

	const deviceCodeTTL = 10 * time.Minute
	expires := time.Now().Add(deviceCodeTTL).Unix()
	deviceCode, userCode, err := h.users.CreateDeviceAuth(expires, req.TokenExpires, req.Scopes)
	if err != nil {
		return EchoError(c, err, http.StatusBadRequest, err.Error())
	}

	baseURL := c.Scheme() + "://" + c.Request().Host
	verificationURI := baseURL + "/auth/activate"
	verificationURIComplete := fmt.Sprintf("%s?user_code=%s", verificationURI, userCode)

	return c.JSON(http.StatusOK, DeviceCodeResponse{
		DeviceCode:              deviceCode,
		UserCode:                userCode,
		VerificationURI:         verificationURI,
		VerificationURIComplete: verificationURIComplete,
		ExpiresIn:               int(deviceCodeTTL.Seconds()),
		Interval:                5, // Poll every 5 seconds
	})
}

// @Summary Handle OAuth2 Device Token Polling
// @Accept json
// @Param data body DeviceTokenRequest true "Device token request"
// @Produce json
// @Success 200
// @Router  /oauth2/device/token [post]
func (h oauth2Handlers) oauth2DeviceToken(c echo.Context) error {
	var req DeviceTokenRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, oauth2Error{
			Error:            "invalid_request",
			ErrorDescription: "Invalid request body",
		})
	}

	if req.GrantType != "urn:ietf:params:oauth:grant-type:device_code" {
		return c.JSON(http.StatusBadRequest, oauth2Error{
			Error:            "unsupported_grant_type",
			ErrorDescription: "Only device_code grant type is supported",
		})
	}

	if req.DeviceCode == "" {
		return c.JSON(http.StatusBadRequest, oauth2Error{
			Error:            "invalid_request",
			ErrorDescription: "device_code is required",
		})
	}

	auth, err := h.users.GetDeviceAuthByDeviceCode(req.DeviceCode)
	if err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to get authorization")
	}
	if auth == nil {
		return c.JSON(http.StatusBadRequest, oauth2Error{
			Error:            "invalid_grant",
			ErrorDescription: "Invalid device code",
		})
	}

	if auth.ExpiresAt < time.Now().Unix() {
		return c.JSON(http.StatusBadRequest, oauth2Error{
			Error:            "invalid_grant",
			ErrorDescription: "Invalid device code",
		})
	}

	if auth.Denied {
		return c.JSON(http.StatusBadRequest, oauth2Error{
			Error:            "access_denied",
			ErrorDescription: "User denied the authorization request",
		})
	}

	if !auth.Authorized {
		return c.JSON(http.StatusBadRequest, oauth2Error{
			Error:            "authorization_pending",
			ErrorDescription: "User has not yet authorized this device",
		})
	}

	if auth.UserID == nil {
		return c.JSON(http.StatusBadRequest, oauth2Error{
			Error:            "invalid_grant",
			ErrorDescription: "No user associated with this authorization",
		})
	}

	user, err := h.users.GetByID(*auth.UserID)
	if err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to get user for authorization")
	}
	if user == nil {
		return c.JSON(http.StatusBadRequest, oauth2Error{
			Error:            "invalid_grant",
			ErrorDescription: "User for authorization not found",
		})
	}

	scopes, err := users.ScopesFromString(auth.Scopes)
	if err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to parse scopes")
	}

	if len(auth.TokenDescription) == 0 {
		auth.TokenDescription = "OAuth2 authorization"
	}
	scopes = user.AllowedScopes & scopes

	// Delete before generating: if generation fails the user must re-authorize,
	// but we avoid issuing a second token if deletion fails after a successful insert.
	if err := h.users.DeleteDeviceAuth(auth.DeviceCode); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to consume device authorization")
	}

	token, err := user.GenerateToken(auth.TokenDescription, auth.TokenExpires, scopes)
	if err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to generate token")
	}

	return c.JSON(http.StatusOK, DeviceTokenResponse{
		AccessToken: token.Value,
		TokenType:   "Bearer",
		Expires:     auth.TokenExpires,
		Scopes:      scopes.String(),
	})
}

type DeviceApproveRequest struct {
	UserCode    string `json:"user_code"`
	Description string `json:"description"`
	// Scopes, when set, narrows the grant further, e.g. to devices:create.
	Scopes string `json:"scopes,omitempty"`
}

const noScopeLimit = ^users.Scopes(0)

type DeviceDenyRequest struct {
	UserCode string `json:"user_code"`
}

type DeviceAuthInfo struct {
	UserCode     string `json:"user_code"`
	Scopes       string `json:"scopes"` // the scopes approving would grant
	TokenExpires int64  `json:"token_expires"`
}

// pendingDeviceAuth looks up a user code that can still be approved or denied.
func (h handlers) pendingDeviceAuth(c echo.Context, userCode string) (*users.OAuth2DeviceAuth, error) {
	if userCode == "" {
		return nil, EchoError(c, nil, http.StatusBadRequest, "user_code is required")
	}
	auth, err := h.users.GetDeviceAuthByUserCode(userCode)
	if err != nil {
		return nil, EchoError(c, err, http.StatusInternalServerError, "Failed to get device authorization")
	}
	if auth == nil {
		return nil, EchoError(c, nil, http.StatusNotFound, "invalid user code")
	}
	if auth.ExpiresAt < time.Now().Unix() {
		return nil, EchoError(c, nil, http.StatusBadRequest, "authorization code expired")
	}
	if auth.Authorized || auth.Denied {
		return nil, EchoError(c, nil, http.StatusBadRequest, "this device has already been authorized or denied")
	}
	return auth, nil
}

// grantableScopes are the requested scopes the caller holds, so approving
// never grants more than the caller could use itself.
func grantableScopes(c echo.Context, auth *users.OAuth2DeviceAuth, limit users.Scopes) (users.Scopes, error) {
	scopes, err := users.ScopesFromString(auth.Scopes)
	if err != nil {
		return 0, EchoError(c, err, http.StatusInternalServerError, "Failed to parse requested scopes: "+auth.Scopes)
	}
	held := scopes & CtxGetUser(c.Request().Context()).AllowedScopes
	if held == 0 {
		return 0, EchoError(c, nil, http.StatusForbidden, fmt.Sprintf("user is missing required scope(s): %s", scopes))
	}
	if held&limit == 0 {
		return 0, EchoError(c, nil, http.StatusBadRequest, fmt.Sprintf("user code does not request scope(s): %s", limit))
	}
	return held & limit, nil
}

// @Summary Show a pending OAuth2 device authorization
// @Param user_code path string true "User code"
// @Produce json
// @Success 200 {object} DeviceAuthInfo
// @Router  /oauth2/device/{user_code} [get]
func (h handlers) oauth2DeviceGet(c echo.Context) error {
	auth, err := h.pendingDeviceAuth(c, c.Param("user_code"))
	if err != nil {
		return err
	}
	granted, err := grantableScopes(c, auth, noScopeLimit)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, DeviceAuthInfo{
		UserCode:     auth.UserCode,
		Scopes:       granted.String(),
		TokenExpires: auth.TokenExpires,
	})
}

// @Summary Approve a pending OAuth2 device authorization
// @Description Approves a user code, as the browser confirmation page does.
// @Description The issued token gets the requested scopes the caller also
// @Description holds, narrowed to the request's scopes when it sets them.
// @Accept json
// @Param data body DeviceApproveRequest true "User code to approve"
// @Success 200
// @Router  /oauth2/device/approve [post]
func (h handlers) oauth2DeviceApprove(c echo.Context) error {
	var req DeviceApproveRequest
	if err := c.Bind(&req); err != nil {
		return EchoError(c, err, http.StatusBadRequest, "Invalid request body")
	}
	limit := noScopeLimit
	if req.Scopes != "" {
		var err error
		if limit, err = users.ScopesFromString(req.Scopes); err != nil {
			return EchoError(c, err, http.StatusBadRequest, "Invalid scopes: "+req.Scopes)
		}
	}
	auth, err := h.pendingDeviceAuth(c, req.UserCode)
	if err != nil {
		return err
	}
	granted, err := grantableScopes(c, auth, limit)
	if err != nil {
		return err
	}

	user := CtxGetUser(c.Request().Context())
	if err := user.ApproveAuthorization(auth.DeviceCode, req.Description, granted); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to approve device authorization")
	}
	return c.NoContent(http.StatusOK)
}

// @Summary Deny a pending OAuth2 device authorization
// @Accept json
// @Param data body DeviceDenyRequest true "User code to deny"
// @Success 200
// @Router  /oauth2/device/deny [post]
func (h handlers) oauth2DeviceDeny(c echo.Context) error {
	var req DeviceDenyRequest
	if err := c.Bind(&req); err != nil {
		return EchoError(c, err, http.StatusBadRequest, "Invalid request body")
	}
	auth, err := h.pendingDeviceAuth(c, req.UserCode)
	if err != nil {
		return err
	}
	user := CtxGetUser(c.Request().Context())
	if err := user.DenyDeviceAuth(auth.DeviceCode); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to deny device authorization")
	}
	return c.NoContent(http.StatusOK)
}
