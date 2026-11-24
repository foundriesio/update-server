// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAuditAppendEvents(t *testing.T) {
	fs, err := NewFs(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(fs.Close)

	start := time.Now().Truncate(time.Second)
	fs.Audit.AppendEvent(42, "User created")
	fs.Audit.AppendEvent(7, "OAuth2 device denied")
	fs.Audit.AppendEvent(42, "Token created: café")
	end := time.Now()

	for _, tc := range []struct {
		userID int64
		file   string
		events []string
	}{
		{42, "users-42", []string{"User created", "Token created: café"}},
		{7, "users-7", []string{"OAuth2 device denied"}},
	} {
		t.Run(tc.file, func(t *testing.T) {
			data, err := fs.Audit.ReadEvents(tc.userID)
			require.NoError(t, err)
			require.True(t, strings.HasSuffix(data, "\n"), "missing final newline: %q", data)
			lines := strings.Split(strings.TrimSuffix(data, "\n"), "\n")
			require.Len(t, lines, len(tc.events))
			for i, line := range lines {
				timestamp, event, found := strings.Cut(line, ": ")
				require.True(t, found, "missing timestamp separator: %q", line)
				require.Equal(t, tc.events[i], event)
				at, err := time.Parse(time.RFC3339, timestamp)
				require.NoError(t, err)
				require.False(t, at.Before(start), "timestamp predates append: %s", timestamp)
				require.False(t, at.After(end), "timestamp follows append: %s", timestamp)
			}

			disk, err := os.ReadFile(filepath.Join(fs.Config.AuditDir(), tc.file))
			require.NoError(t, err)
			require.Equal(t, data, string(disk))
		})
	}
}

func TestAuditReadEventsMissing(t *testing.T) {
	fs, err := NewFs(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(fs.Close)

	data, err := fs.Audit.ReadEvents(42)
	require.Empty(t, data)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.ErrorContains(t, err, "reading audit log for user 42:")
}
