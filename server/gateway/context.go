// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package gateway

import (
	"github.com/foundriesio/update-server/context"
	storage "github.com/foundriesio/update-server/storage/gateway"
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
	ctxKeyDevice ctxKey = iota
)

func CtxGetDevice(ctx context.Context) *storage.Device {
	return ctx.Value(ctxKeyDevice).(*storage.Device)
}

func CtxWithDevice(ctx Context, device *storage.Device) Context {
	return context.WithValue(ctx, ctxKeyDevice, device)
}
