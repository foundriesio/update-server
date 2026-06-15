// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package auth

import (
	"net/http"

	"github.com/foundriesio/update-server/storage/users"
)

type User interface {
	Id() string
	Scopes() users.Scopes
}

// AuthUserFunc allows us to define a generic way for middleware to do
// authentication and authorization based on the incoming http request.
// The function returns nil if the user wasn't authenticated implying
// this function returned the proper error to the caller.
type AuthUserFunc func(w http.ResponseWriter, r *http.Request) (User, error)
