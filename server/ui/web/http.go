// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/foundriesio/dg-satellite/auth"
	"github.com/foundriesio/dg-satellite/context"
)

func getJson(ctx context.Context, resource string, result any) error {
	_, err := getJsonWithHeaders(ctx, resource, result)
	return err
}

func putJson(ctx context.Context, resource string, data, result any) error {
	_, err := putJsonWithHeaders(ctx, resource, data, result)
	return err
}

func getJsonWithHeaders(ctx context.Context, resource string, result any) (http.Header, error) {
	s := CtxGetSession(ctx)
	req, err := http.NewRequest("GET", s.BaseUrl+resource, nil)
	if err != nil {
		return nil, err
	}
	return reqJsonWithHeaders(ctx, req, result)
}

func putJsonWithHeaders(ctx context.Context, resource string, data, result any) (http.Header, error) {
	s := CtxGetSession(ctx)
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(data); err != nil {
		return nil, err
	}
	req, err := http.NewRequest("PUT", s.BaseUrl+resource, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	auth.PassCsrfCookie(ctx, req)
	return reqJsonWithHeaders(ctx, req, result)
}

func reqJsonWithHeaders(ctx context.Context, req *http.Request, result any) (http.Header, error) {
	s := CtxGetSession(ctx)
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			context.CtxGetLog(ctx).Error("unable to close response body", "error", err)
		}
	}()

	switch resp.StatusCode {
	case http.StatusOK:
		if result == nil {
			// Caller requested to skip result data
			_, _ = io.Copy(io.Discard, resp.Body)
			return resp.Header, nil
		}
		return resp.Header, json.NewDecoder(resp.Body).Decode(result)
	case http.StatusNoContent:
		return resp.Header, nil
	default:
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: HTTP_%d: %s", resp.StatusCode, string(body))
	}
}
