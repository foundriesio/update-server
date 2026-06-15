// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/foundriesio/update-server/storage"
)

func (d *Device) TestCreate(targetName string, testName, testId string) error {
	t := storage.TargetTest{
		Uuid:       testId,
		Name:       testName,
		TargetName: targetName,
		Status:     "RUNNING",
		CreatedOn:  time.Now().UTC().Unix(),
	}
	testBytes, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("unexpected error marshaling test json: %w", err)
	}
	return d.storage.fs.Devices.WriteFile(d.Uuid, fmt.Sprintf("%s%s", storage.TestsPrefix, testId), string(testBytes))
}

func (d *Device) TestComplete(testId, status, details string, results []storage.TargetTestResult) error {
	if status != "PASSED" && status != "FAILED" {
		return fmt.Errorf("invalid test status: %s. Must be PASSED or FAILED", status)
	}

	var t storage.TargetTest
	bytes, err := d.storage.fs.Devices.ReadFile(d.Uuid, fmt.Sprintf("%s%s", storage.TestsPrefix, testId))
	if err != nil {
		return fmt.Errorf("error reading test data for %s: %w", testId, err)
	}
	if err := json.Unmarshal([]byte(bytes), &t); err != nil {
		return fmt.Errorf("error unmarshalling test data for %s: %w", testId, err)
	}

	for _, res := range results {
		if res.Status != "PASSED" && res.Status != "FAILED" && res.Status != "SKIPPED" {
			return fmt.Errorf("invalid test-result status: %s. Must be PASSED, FAILED, or SKIPPED", res.Status)
		}
	}

	ts := time.Now().UTC().Unix()
	t.Status = status
	t.Details = details
	t.CompletedOn = &ts
	t.Results = results

	testBytes, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("unexpected error marshaling test json: %w", err)
	}

	if err := d.storage.fs.Devices.WriteFile(d.Uuid, fmt.Sprintf("%s%s", storage.TestsPrefix, testId), string(testBytes)); err != nil {
		return fmt.Errorf("failed to save completed test data for %s: %w", testId, err)
	}
	return nil
}

func (d *Device) TestStoreArtifact(testId, path string, body io.Reader) error {
	name := fmt.Sprintf("%s%s", storage.TestsPrefix, testId)
	if content, err := d.storage.fs.Devices.ReadFile(d.Uuid, name); err != nil || len(content) == 0 {
		return fmt.Errorf("test with id %s does not exist", testId)
	}
	name = fmt.Sprintf("%s-%s_%s", storage.TestArtifactsPrefix, testId, path)
	if _, err := storage.AbsPathNoEscape("/", path); err != nil {
		return fmt.Errorf("invalid artifact path: %w", err)
	}
	return d.storage.fs.Devices.WriteFileStream(d.Uuid, name, body)
}
