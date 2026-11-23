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

	"github.com/foundriesio/update-server/server/ui/i18n"
	"github.com/foundriesio/update-server/storage/users"
	"github.com/labstack/echo/v4"
)

type settingsScopeGroup struct {
	Resource   string
	Operations string
	ScopeNames []string
}

type settingsScopeGroupBuilder struct {
	resource   string
	operations map[string]struct{}
	scopeNames []string
}

var settingsScopeOperationOrder = map[string]int{
	scopeOperationCreate: 0,
	scopeOperationDelete: 1,
	scopeOperationRead:   2,
	"update":             3,
	"unrecognized":       4,
}

func groupSettingsScopes(scopes []string) []settingsScopeGroup {
	grouped := make(map[string]int)
	builders := make([]settingsScopeGroupBuilder, 0, len(scopes))
	for _, scopeName := range scopes {
		resource, operation, valid := strings.Cut(scopeName, ":")
		if !valid || resource == "" || operation == "" {
			if resource == "" {
				resource = scopeName
			}
			if resource == "" {
				resource = "(empty)"
			}
			operation = ""
		}

		groupIndex, ok := grouped[resource]
		if !ok {
			groupIndex = len(builders)
			grouped[resource] = groupIndex
			builders = append(builders, settingsScopeGroupBuilder{
				resource:   resource,
				operations: make(map[string]struct{}),
			})
		}
		group := &builders[groupIndex]
		group.scopeNames = append(group.scopeNames, scopeName)
		for _, summary := range settingsScopeOperations(operation, valid) {
			group.operations[summary] = struct{}{}
		}
	}

	groups := make([]settingsScopeGroup, 0, len(builders))
	for _, group := range builders {
		groups = append(groups, settingsScopeGroup{
			Resource:   group.resource,
			Operations: settingsScopeOperationSummary(group.operations),
			ScopeNames: group.scopeNames,
		})
	}
	return groups
}

func settingsScopeOperations(operation string, valid bool) []string {
	if !valid || operation == "" {
		return []string{"unrecognized"}
	}
	switch operation {
	case scopeOperationRead:
		return []string{scopeOperationRead}
	case "read-update":
		return []string{scopeOperationRead, "update"}
	case scopeOperationCreate:
		return []string{scopeOperationCreate}
	case scopeOperationDelete:
		return []string{scopeOperationDelete}
	default:
		return []string{strings.ReplaceAll(strings.ReplaceAll(operation, "-", " "), "_", " ")}
	}
}

func settingsScopeOperationSummary(operations map[string]struct{}) string {
	summary := make([]string, 0, len(operations))
	for operation := range operations {
		summary = append(summary, operation)
	}
	sort.Slice(summary, func(i, j int) bool {
		leftOrder, leftKnown := settingsScopeOperationOrder[summary[i]]
		rightOrder, rightKnown := settingsScopeOperationOrder[summary[j]]
		if leftKnown != rightKnown {
			return leftKnown
		}
		if leftKnown {
			return leftOrder < rightOrder
		}
		return summary[i] < summary[j]
	})

	if len(summary) == 0 {
		return "Unrecognized"
	}
	if len(summary) == 1 {
		return settingsScopeSentenceCase(summary[0])
	}
	return settingsScopeSentenceCase(strings.Join(summary[:len(summary)-1], ", ")) + " or " + summary[len(summary)-1]
}

func settingsScopeSentenceCase(value string) string {
	return strings.ToUpper(value[:1]) + value[1:]
}

func (h handlers) settings(c echo.Context) error {
	session := CtxGetSession(c.Request().Context())
	tokens, err := session.User.ListTokens()
	if err != nil {
		return h.handleUnexpected(c, err)
	}

	scopes := session.User.AllowedScopes.ToSlice()
	base := h.baseCtx(c, "Settings", "settings")
	storedLocale := session.User.Locale
	if _, ok := i18n.Valid(storedLocale); !ok {
		storedLocale = ""
	}
	effectiveLanguage := ""
	for _, lang := range i18n.Languages() {
		if lang.Tag == base.L.Tag() {
			effectiveLanguage = lang.Name
			break
		}
	}
	ctx := struct {
		baseCtx
		Tokens            []users.Token
		ScopesList        []string
		GrantableScopes   []string
		ScopeGroups       []settingsScopeGroup
		LocalAuth         bool
		DeviceApiUrl      string
		Oauth2ApiUrl      string
		Languages         []i18n.Language
		StoredLocale      string
		EffectiveLanguage string
	}{
		baseCtx:           base,
		Tokens:            tokens,
		ScopesList:        scopes,
		GrantableScopes:   session.User.AllowedScopes.Grantable(),
		ScopeGroups:       groupSettingsScopes(scopes),
		LocalAuth:         h.provider.Name() == "local",
		DeviceApiUrl:      c.Scheme() + "://" + c.Request().Host + "/v1/devices",
		Oauth2ApiUrl:      c.Scheme() + "://" + c.Request().Host + "/oauth2",
		Languages:         i18n.Languages(),
		StoredLocale:      storedLocale,
		EffectiveLanguage: effectiveLanguage,
	}
	return c.Render(http.StatusOK, "settings.html", ctx)
}

func (h handlers) settingsTokenUpdate(c echo.Context) error {
	session := CtxGetSession(c.Request().Context())
	tokenID, err := strconv.ParseInt(c.Param("tokenID"), 10, 64)
	if err != nil {
		return EchoError(c, err, http.StatusBadRequest, "Invalid token ID format")
	}

	type TokenUpdateRequest struct {
		Description string   `json:"description"`
		Scopes      []string `json:"scopes"`
	}
	var req TokenUpdateRequest
	if err := c.Bind(&req); err != nil {
		return EchoError(c, err, http.StatusBadRequest, "Could not parse request")
	}

	req.Description = strings.TrimSpace(req.Description)
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

	if err := session.User.UpdateToken(tokenID, req.Description, scopes); err != nil {
		return EchoError(c, err, http.StatusBadRequest, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}
