// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"github.com/foundriesio/update-server/auth"
	"github.com/foundriesio/update-server/context"
)

type ctxKey int

const ctxKeySession ctxKey = iota

func CtxGetSession(ctx context.Context) *auth.Session {
	return ctx.Value(ctxKeySession).(*auth.Session)
}

func CtxWithSession(ctx context.Context, session *auth.Session) context.Context {
	return context.WithValue(ctx, ctxKeySession, session)
}
