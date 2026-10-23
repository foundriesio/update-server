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
	"unicode"
	"unicode/utf8"

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
	scopeOperationCreate:       0,
	scopeOperationDelete:       1,
	scopeOperationRead:         2,
	scopeOperationUpdate:       3,
	scopeOperationUnrecognized: 4,
}

func groupSettingsScopes(scopes []string, loc i18n.Printer) []settingsScopeGroup {
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
			Operations: settingsScopeOperationSummary(group.operations, loc),
			ScopeNames: group.scopeNames,
		})
	}
	return groups
}

func settingsScopeOperations(operation string, valid bool) []string {
	if !valid || operation == "" {
		return []string{scopeOperationUnrecognized}
	}
	switch operation {
	case scopeOperationRead:
		return []string{scopeOperationRead}
	case "read-update":
		return []string{scopeOperationRead, scopeOperationUpdate}
	case scopeOperationCreate:
		return []string{scopeOperationCreate}
	case scopeOperationDelete:
		return []string{scopeOperationDelete}
	default:
		return []string{strings.ReplaceAll(strings.ReplaceAll(operation, "-", " "), "_", " ")}
	}
}

// settingsScopeOperationLabel returns the translated word for a known operation key.
// Raw operations from unknown scopes stay as the scope wrote them.
func settingsScopeOperationLabel(operation string, loc i18n.Printer) string {
	switch operation {
	case scopeOperationCreate:
		return loc.T("create")
	case scopeOperationDelete:
		return loc.T("delete")
	case scopeOperationRead:
		return loc.T("read")
	case scopeOperationUpdate:
		return loc.T("update")
	case scopeOperationUnrecognized:
		return loc.T("unrecognized")
	}
	return operation
}

// settingsScopeOperationSummary joins the labels as "a, b or c". The fold keeps
// each msgid literal, so translators control the separators per language.
func settingsScopeOperationSummary(operations map[string]struct{}, loc i18n.Printer) string {
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

	labels := make([]string, 0, len(summary))
	for _, operation := range summary {
		labels = append(labels, settingsScopeOperationLabel(operation, loc))
	}
	if len(labels) == 1 {
		return settingsScopeSentenceCase(labels[0])
	}

	// summary is never empty: settingsScopeOperations always returns at least one key.
	joined := labels[0]
	for _, label := range labels[1 : len(labels)-1] {
		joined = loc.T("%[1]s, %[2]s", joined, label)
	}
	return settingsScopeSentenceCase(loc.T("%[1]s or %[2]s", joined, labels[len(labels)-1]))
}

func settingsScopeSentenceCase(value string) string {
	first, size := utf8.DecodeRuneInString(value)
	return string(unicode.ToUpper(first)) + value[size:]
}

func (h handlers) settings(c echo.Context) error {
	session := CtxGetSession(c.Request().Context())
	tokens, err := session.User.ListTokens()
	if err != nil {
		return h.handleUnexpected(c, err)
	}

	scopes := session.User.AllowedScopes.ToSlice()
	base := h.baseCtx(c, "Settings", "settings")
	// <title> and the h1 read Title, which must be the translated name.
	base.Title = base.L.T("Settings")
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
		ScopeGroups:       groupSettingsScopes(scopes, base.L),
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
