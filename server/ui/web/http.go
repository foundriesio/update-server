// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/foundriesio/update-server/auth"
	"github.com/foundriesio/update-server/context"
)

func getJson(ctx context.Context, resource string, result any) error {
	_, err := getJsonWithHeaders(ctx, resource, result)
	return err
}

func putJson(ctx context.Context, resource string, data, result any) error {
	_, err := putJsonWithHeaders(ctx, resource, data, result)
	return err
}

func postJson(ctx context.Context, resource string, data, result any) error {
	_, err := sendJsonWithHeaders(ctx, http.MethodPost, resource, data, result)
	return err
}

func getJsonWithHeaders(ctx context.Context, resource string, result any) (http.Header, error) {
	s := CtxGetSession(ctx)
	req, err := http.NewRequest(http.MethodGet, s.BaseUrl+resource, nil)
	if err != nil {
		return nil, err
	}
	return reqJsonWithHeaders(ctx, req, result)
}

func putJsonWithHeaders(ctx context.Context, resource string, data, result any) (http.Header, error) {
	return sendJsonWithHeaders(ctx, http.MethodPut, resource, data, result)
}

func sendJsonWithHeaders(ctx context.Context, method, resource string, data, result any) (http.Header, error) {
	s := CtxGetSession(ctx)
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(data); err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, s.BaseUrl+resource, &body)
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
		return nil, &apiStatusError{Status: resp.StatusCode, Body: body}
	}
}

// apiStatusError lets a handler show the API's own status and message.
type apiStatusError struct {
	Status int
	Body   []byte
}

func (e *apiStatusError) Error() string {
	return fmt.Sprintf("unexpected status code: HTTP_%d: %s", e.Status, string(e.Body))
}

// Message is the API's error message, or the raw body if it isn't JSON.
func (e *apiStatusError) Message() string {
	var msg struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(e.Body, &msg) == nil && msg.Message != "" {
		return msg.Message
	}
	return string(e.Body)
}
