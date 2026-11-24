// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"fmt"
	"log/slog"
	"time"
)

type AuditLogsFsHandle struct {
	osRootHandle
}

func (h AuditLogsFsHandle) AppendEvent(userid int64, event string) {
	msg := fmt.Sprintf("%s: %s\n", time.Now().Format(time.RFC3339), event)
	if err := h.appendFile(fmt.Sprintf("users-%d", userid), msg, defaultFileAccess); err != nil {
		slog.Error("Failed to append audit log", "userID", userid, "error", err)
	}
}

func (h AuditLogsFsHandle) ReadEvents(userid int64) (string, error) {
	data, err := h.readFile(fmt.Sprintf("users-%d", userid), false)
	if err != nil {
		return "", fmt.Errorf("reading audit log for user %d: %w", userid, err)
	}
	return data, nil
}
