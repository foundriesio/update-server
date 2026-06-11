// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/foundriesio/update-server/storage"
)

type TargetTest = storage.TargetTest

func (d *Device) GetTests() ([]TargetTest, error) {
	var tests []TargetTest
	files, err := d.storage.fs.Devices.ListFiles(d.Uuid, storage.TestsPrefix, true)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return []TargetTest{}, nil
	} else if err != nil {
		return nil, err
	}

	for _, file := range files {
		testData, err := d.storage.fs.Devices.ReadFile(d.Uuid, file)
		if err != nil {
			return nil, err
		}
		var test storage.TargetTest
		if err := json.Unmarshal([]byte(testData), &test); err != nil {
			return nil, fmt.Errorf("unexpected error unmarshalling %s: %w", file, err)
		}
		tests = append(tests, test)
	}
	return tests, nil
}

func (d *Device) GetTest(testId string) (*TargetTest, error) {
	name := fmt.Sprintf("%s%s", storage.TestsPrefix, testId)
	var test storage.TargetTest
	bytes, err := d.storage.fs.Devices.ReadFile(d.Uuid, name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("error reading test %s: %w", testId, err)
	}
	if err := json.Unmarshal([]byte(bytes), &test); err != nil {
		return nil, fmt.Errorf("error unmarshalling test %s: %w", testId, err)
	}

	prefix := fmt.Sprintf("%s-%s_", storage.TestArtifactsPrefix, testId)
	files, err := d.storage.fs.Devices.ListFiles(d.Uuid, prefix, true)
	if err != nil {
		return nil, fmt.Errorf("error listing artifacts for test %s: %w", testId, err)
	}
	for i, file := range files {
		files[i] = file[len(prefix):]
	}
	test.Artifacts = files
	return &test, nil
}

func (d *Device) GetTestArtifact(testId, name string) (io.ReadCloser, error) {
	name = fmt.Sprintf("%s-%s_%s", storage.TestArtifactsPrefix, testId, name)
	return d.storage.fs.Devices.ReadFileStream(d.Uuid, name)
}
