// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"github.com/foundriesio/update-server/context"
	"github.com/foundriesio/update-server/storage/users"
)

type (
	Context = context.Context
	ctxKey  int
)

var (
	CtxGetLog  = context.CtxGetLog
	CtxWithLog = context.CtxWithLog
)

const (
	ctxKeyUser ctxKey = iota
)

func CtxGetUser(ctx Context) *users.User {
	return ctx.Value(ctxKeyUser).(*users.User)
}

func CtxWithUser(ctx Context, user *users.User) Context {
	return context.WithValue(ctx, ctxKeyUser, user)
}
