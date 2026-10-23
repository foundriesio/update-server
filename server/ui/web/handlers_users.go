// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/foundriesio/update-server/storage/users"
	"github.com/labstack/echo/v4"
)

type usersDirectoryCtx struct {
	baseCtx
	Entries       []userDirectoryEntry
	ScopesList    []string
	CanCreateUser bool
	IsLocalAuth   bool
	AuthProvider  string
}

type directoryPermissions struct {
	IsLocalAuth     bool
	CanCreateUser   bool
	CanManageScopes bool
	CanDelete       bool
}

type scopeGroup struct {
	Resource     string
	Capabilities []string
}

type userDirectoryEntry struct {
	Username         string
	Email            string
	CreatedAt        int64
	Scopes           []string
	ScopeGroups      []scopeGroup
	ResourceCount    int
	IsCurrent        bool
	CanManageScopes  bool
	CanResetPassword bool
	CanDelete        bool
}

func (h handlers) usersList(c echo.Context) error {
	userList, err := h.users.List()
	if err != nil {
		return h.handleUnexpected(c, err)
	}
	session := CtxGetSession(c.Request().Context())
	viewerScopes := session.User.AllowedScopes
	providerName := h.provider.Name()
	permissions := directoryPermissionsFor(providerName, viewerScopes)
	ctx := usersDirectoryCtx{
		baseCtx:       h.baseCtx(c, usersLabel, "users"),
		Entries:       buildUserDirectoryEntries(userList, session.User.Username, permissions),
		ScopesList:    users.ScopesAvailable(),
		CanCreateUser: permissions.CanCreateUser,
		IsLocalAuth:   permissions.IsLocalAuth,
		AuthProvider:  providerName,
	}
	return h.templates.ExecuteTemplate(c.Response(), "users.html", ctx)
}

func directoryPermissionsFor(providerName string, sessionScopes users.Scopes) directoryPermissions {
	isLocalAuth := providerName == "local"
	return directoryPermissions{
		IsLocalAuth:     isLocalAuth,
		CanCreateUser:   isLocalAuth && sessionScopes.Has(users.ScopeUsersC),
		CanManageScopes: sessionScopes.Has(users.ScopeUsersRU),
		CanDelete:       sessionScopes.Has(users.ScopeUsersD),
	}
}

func buildUserDirectoryEntries(list []users.User, currentUsername string, permissions directoryPermissions) []userDirectoryEntry {
	entries := make([]userDirectoryEntry, 0, len(list))
	for _, user := range list {
		scopes := user.AllowedScopes.ToSlice()
		groups := groupScopes(scopes)
		isCurrent := user.Username == currentUsername
		entries = append(entries, userDirectoryEntry{
			Username:         user.Username,
			Email:            user.Email,
			CreatedAt:        user.CreatedAt,
			Scopes:           scopes,
			ScopeGroups:      groups,
			ResourceCount:    len(groups),
			IsCurrent:        isCurrent,
			CanManageScopes:  permissions.CanManageScopes,
			CanResetPassword: permissions.IsLocalAuth && permissions.CanManageScopes && !isCurrent,
			CanDelete:        permissions.CanDelete && !isCurrent,
		})
	}
	return entries
}

var scopeResourceOrder = map[string]int{
	devicesLabel: 0,
	updatesLabel: 1,
	usersLabel:   2,
}

var scopeCapabilityOrder = map[string]int{
	"Read + update": 0,
	"Read":          1,
	"Create":        2,
	"Delete":        3,
}

func groupScopes(scopes []string) []scopeGroup {
	grouped := make(map[string][]string)
	for _, scope := range scopes {
		resource, capability, ok := strings.Cut(scope, ":")
		if !ok || resource == "" || capability == "" {
			continue
		}

		resource = scopeResourceLabel(resource)
		grouped[resource] = append(grouped[resource], scopeCapabilityLabel(capability))
	}

	groups := make([]scopeGroup, 0, len(grouped))
	for resource, capabilities := range grouped {
		sort.SliceStable(capabilities, func(i, j int) bool {
			leftOrder, leftKnown := scopeCapabilityOrder[capabilities[i]]
			rightOrder, rightKnown := scopeCapabilityOrder[capabilities[j]]
			if leftKnown != rightKnown {
				return leftKnown
			}
			if leftKnown {
				return leftOrder < rightOrder
			}
			return capabilities[i] < capabilities[j]
		})
		groups = append(groups, scopeGroup{Resource: resource, Capabilities: capabilities})
	}

	sort.SliceStable(groups, func(i, j int) bool {
		leftOrder, leftKnown := scopeResourceOrder[groups[i].Resource]
		rightOrder, rightKnown := scopeResourceOrder[groups[j].Resource]
		if leftKnown != rightKnown {
			return leftKnown
		}
		if leftKnown {
			return leftOrder < rightOrder
		}
		return groups[i].Resource < groups[j].Resource
	})
	return groups
}

func scopeResourceLabel(resource string) string {
	switch resource {
	case "devices":
		return devicesLabel
	case "updates":
		return updatesLabel
	case "users":
		return usersLabel
	default:
		first, size := utf8.DecodeRuneInString(resource)
		return string(unicode.ToUpper(first)) + resource[size:]
	}
}

func scopeCapabilityLabel(capability string) string {
	switch capability {
	case "read":
		return "Read"
	case "read-update":
		return "Read + update"
	case "create":
		return "Create"
	case "delete":
		return "Delete"
	default:
		return capability
	}
}

func (h handlers) userDelete(c echo.Context) error {
	session := CtxGetSession(c.Request().Context())
	username := c.Param("username")
	user, err := h.users.Get(username)
	if err != nil {
		return h.handleError(c, http.StatusNotFound, err)
	}

	if session.User.Username == user.Username {
		err := errors.New("users cannot delete themselves")
		return EchoError(c, err, http.StatusBadRequest, err.Error())
	}

	if err := user.Delete(); err != nil {
		return h.handleUnexpected(c, err)
	}
	return nil
}

func (h handlers) usersAuditLog(c echo.Context) error {
	username := c.Param("username")
	user, err := h.users.Get(username)
	if err != nil {
		return h.handleError(c, http.StatusNotFound, err)
	}

	log, err := user.GetAuditLog()
	if err != nil {
		return h.handleUnexpected(c, err)
	}
	return c.String(http.StatusOK, log)
}
func (h *handlers) userScopesUpdate(c echo.Context) error {
	session := CtxGetSession(c.Request().Context())
	// Check this scope here rather than middleware so that we can return a JSON error
	// to the JS caller.
	if !session.User.AllowedScopes.Has(users.ScopeUsersRU) {
		err := fmt.Errorf("user missing required scope: %s", users.ScopeUsersRU)
		return EchoError(c, err, http.StatusForbidden, err.Error())
	}

	type request struct {
		Scopes []string `json:"scopes"`
	}
	var req request
	if err := c.Bind(&req); err != nil {
		return EchoError(c, err, http.StatusBadRequest, "Could not parse request")
	}

	if len(req.Scopes) == 0 {
		err := errors.New("at least one scope is required")
		return EchoError(c, err, http.StatusBadRequest, err.Error())
	}

	scopes, err := users.ScopesFromSlice(req.Scopes)
	if err != nil {
		return EchoError(c, err, http.StatusBadRequest, fmt.Sprintf("Invalid scope: %s", err))
	}

	username := c.Param("username")
	user, err := h.users.Get(username)
	if err != nil {
		return h.handleError(c, http.StatusNotFound, err)
	}

	user.AllowedScopes = scopes
	if err := user.Update("Scopes changed by " + session.User.Username); err != nil {
		return h.handleUnexpected(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *handlers) userTokenCreate(c echo.Context) error {
	session := CtxGetSession(c.Request().Context())

	type TokenRequest struct {
		Description string   `json:"description"`
		Scopes      []string `json:"scopes"`
		Expires     string   `json:"expires"`
	}
	var req TokenRequest
	if err := c.Bind(&req); err != nil {
		return EchoError(c, err, http.StatusBadRequest, "Could not parse request")
	}

	if len(req.Description) == 0 {
		err := errors.New("token description is required")
		return EchoError(c, err, http.StatusBadRequest, err.Error())
	}
	if len(req.Scopes) == 0 {
		err := errors.New("at least one scope is required")
		return EchoError(c, err, http.StatusBadRequest, err.Error())
	}

	scopes, err := users.ScopesFromSlice(req.Scopes)
	if err != nil {
		return EchoError(c, err, http.StatusBadRequest, fmt.Sprintf("Invalid scope: %s", err))
	}

	// Parse the ISO date string
	expires, err := time.Parse(time.RFC3339, req.Expires)
	if err != nil {
		return EchoError(c, err, http.StatusBadRequest, "Invalid date format. Expected ISO date string. Got"+req.Expires)
	}

	// Validate that the expiration date is in the future
	if expires.Before(time.Now()) {
		err := fmt.Errorf("expiration date must be in the future. Got: %s", req.Expires)
		return EchoError(c, err, http.StatusBadRequest, err.Error())
	}
	tok, err := session.User.GenerateToken(req.Description, expires.Unix(), scopes)
	if err != nil {
		return EchoError(c, err, http.StatusBadRequest, err.Error())
	}
	return c.String(http.StatusCreated, tok.Value)
}

func (h *handlers) userTokenDelete(c echo.Context) error {
	session := CtxGetSession(c.Request().Context())
	tokenIDStr := c.Param("tokenID")
	tokenID, err := strconv.ParseInt(tokenIDStr, 10, 64)
	if err != nil {
		return EchoError(c, err, http.StatusBadRequest, "Invalid token ID format")
	}
	if err := session.User.DeleteToken(tokenID); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to delete token")
	}
	return c.NoContent(http.StatusNoContent)
}
