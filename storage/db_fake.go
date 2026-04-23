// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

//go:build nodb

package storage

import (
	"database/sql"
	"errors"
)

var ErrDbConstraintUnique = errors.New("sqllite.ErrConstraintUnique")

func IsDbError(err error, code any) bool {
	return false
}

type DbHandle struct {
}

func NewDb(dbfile string) (*DbHandle, error) {
	return nil, nil
}

func (d DbHandle) Close() error {
	return nil
}

func (d DbHandle) Query(query string, args ...any) (*sql.Rows, error) {
	return nil, nil
}

func (d DbHandle) QueryRow(query string, args ...any) *sql.Row {
	return nil
}

func (d DbHandle) Prepare(name, query string) (stmt *sql.Stmt, err error) {
	return nil, nil
}

func (d DbHandle) InitStmt(stmt ...DbStmtInit) error {
	return nil
}

type DbStmt struct {
	Stmt *sql.Stmt
}

type DbStmtInit interface {
	Init(db DbHandle) error
}
