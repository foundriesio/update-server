// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package daemons

import (
	"context"
	"time"

	storage "github.com/foundriesio/update-server/storage/api"
	"github.com/foundriesio/update-server/storage/users"
)

type daemonFunc func(stop chan bool)

type Option func(*daemons)

type daemons struct {
	context context.Context
	storage *storage.Storage
	daemons []daemonFunc
	stops   []chan bool

	rolloutOptions rolloutOptions
}

func New(context context.Context, storage *storage.Storage, users *users.Storage, opts ...Option) *daemons {
	d := &daemons{context: context, storage: storage}
	d.rolloutOptions = rolloutOptions{
		interval: 5 * time.Minute,
	}
	d.daemons = []daemonFunc{
		d.rolloutWatchdog(),
		userGcDaemonFunc(users),
	}

	for _, opt := range opts {
		opt(d)
	}
	return d
}

func (d *daemons) Start() {
	for _, f := range d.daemons {
		stop := make(chan bool)
		d.stops = append(d.stops, stop)
		go f(stop)
	}
}

func (d *daemons) Shutdown() {
	for _, s := range d.stops {
		s <- true
	}
}
