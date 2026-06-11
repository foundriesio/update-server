// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package gateway

import (
	"fmt"
	"mime"
	"net/http"
	"path/filepath"

	"github.com/foundriesio/update-server/storage"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type testCreateBody struct {
	Name   string `json:"name"`
	TestId string `json:"test-id"`
}

// @Summary Create a test
// @Accept  json
// @Param   test body testCreateBody true "Test body"
// @Param   x-ats-target header string true "Target name"
// @Produce plain
// @Success 201 "test-id"
// @Router  /tests [post]
func (h handlers) testCreate(c echo.Context) error {
	d := CtxGetDevice(c.Request().Context())
	target := c.Request().Header.Get("x-ats-target")

	var test testCreateBody
	if err := ReadJsonBody(c, &test); err != nil {
		return err
	}

	if len(test.TestId) > 0 && !storage.TestIdRegex.MatchString(test.TestId) {
		msg := fmt.Sprintf("test-id(%s) must match pattern: %s", test.TestId, storage.TestIdRegex.String())
		return EchoError(c, nil, http.StatusBadRequest, msg)
	} else if len(test.TestId) == 0 {
		test.TestId = uuid.New().String()
	}
	if err := d.TestCreate(target, test.Name, test.TestId); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to save test")
	}

	return c.String(http.StatusCreated, test.TestId)
}

type testCompleteBody struct {
	Status    string                     `json:"status"`
	Details   string                     `json:"details"`
	Results   []storage.TargetTestResult `json:"results"`
	Artifacts []string                   `json:"artifacts"`
}

type signedUrl struct {
	Url         string `json:"url"`
	ContentType string `json:"content-type"`
}

// @Summary Complete a test
// @Accept  json
// @Param   test body testCompleteBody true "Test details"
// @Produce json
// @Success 200 {object} map[string]signedUrl
// @Router  /tests/{test-id} [put]
func (h handlers) testComplete(c echo.Context) error {
	ctx := c.Request().Context()
	d := CtxGetDevice(ctx)
	testid := c.Param("testid")

	log := CtxGetLog(ctx)
	log = log.With("testid", testid)
	ctx = CtxWithLog(ctx, log)
	c.SetRequest(c.Request().WithContext(ctx))

	var test testCompleteBody
	if err := ReadJsonBody(c, &test); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to parse request body")
	}

	if test.Status == "" {
		test.Status = "PASSED"
	}

	if err := d.TestComplete(testid, test.Status, test.Details, test.Results); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to save test")
	}

	// NOTE: c.Request().URL doesn't include the base host info of the request
	// so we have use Request().Host
	baseUrl := "https://" + c.Request().Host + c.Request().URL.Path
	urls := make(map[string]signedUrl)
	for _, p := range test.Artifacts {
		urls[p] = signedUrl{
			Url:         baseUrl + "/" + p,
			ContentType: guessContentType(p),
		}
	}

	return c.JSON(http.StatusOK, urls)
}

func (h handlers) testArtifact(c echo.Context) error {
	ctx := c.Request().Context()
	d := CtxGetDevice(ctx)
	testid := c.Param("testid")
	path := c.Param("path")

	log := CtxGetLog(ctx)
	log = log.With("testid", testid)
	ctx = CtxWithLog(ctx, log)
	c.SetRequest(c.Request().WithContext(ctx))

	if err := d.TestStoreArtifact(testid, path, c.Request().Body); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to save test artifact")
	}
	return c.String(http.StatusOK, "OK")
}

func guessContentType(path string) string {
	ext := filepath.Ext(path)
	if ext == ".log" {
		// Ubuntu doesn't do this, so hack
		ext = ".txt"
	}
	ct := mime.TypeByExtension(ext)
	if ct == "" {
		ct = "application/octet-stream"
	}
	return ct
}
