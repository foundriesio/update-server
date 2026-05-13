// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type HttpOption func(opts *httpOptions)

func HttpHeader(name, value string) HttpOption {
	return func(opts *httpOptions) {
		opts.header.Set(name, value)
	}
}

func ParseNextLink(linkHeader string) (string, bool) {
	return parseLinkRel(linkHeader, "next")
}

func ParseLastLink(linkHeader string) (string, bool) {
	return parseLinkRel(linkHeader, "last")
}

func parseLinkRel(linkHeader, rel string) (string, bool) {
	target := `rel="` + rel + `"`
	for _, part := range strings.Split(linkHeader, ",") {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, target) {
			continue
		}
		start := strings.Index(part, "<")
		end := strings.Index(part, ">")
		if start >= 0 && end > start {
			return part[start+1 : end], true
		}
	}
	return "", false
}

func (a Api) GetWithHeaders(resource string, result any, opts ...HttpOption) (http.Header, error) {
	resp, err := a.get(resource, opts...)
	if err != nil {
		return nil, err
	}
	defer a.closeHttpBody(resp.Body)
	if resp.ContentLength != 0 {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return nil, err
		}
	}
	return resp.Header, nil
}

func (a Api) Get(resource string, result any, opts ...HttpOption) error {
	_, err := a.GetWithHeaders(resource, result, opts...)
	return err
}

func (a Api) GetStream(resource string, opts ...HttpOption) (io.ReadCloser, error) {
	resp, err := a.get(resource, opts...)
	return resp.Body, err
}

func (a Api) get(resource string, opts ...HttpOption) (*http.Response, error) {
	var options httpOptions
	options.apply(opts)
	url := a.URL + resource

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header = options.header

	resp, err := a.Client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer a.closeHttpBody(resp.Body)
		return nil, a.handleHttpError(resp)
	}
	// Return the response without closing the body - caller must close it
	return resp, nil
}

func (a Api) Delete(resource string, opts ...HttpOption) error {
	var options httpOptions
	options.apply(opts)
	url := a.URL + resource

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header = options.header

	resp, err := a.Client.Do(req)
	if err != nil {
		return err
	}
	defer a.closeHttpBody(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return a.handleHttpError(resp)
	}
	return nil
}

func (a Api) Post(resource string, body any, opts ...HttpOption) ([]byte, error) {
	var options httpOptions
	options.apply(opts)
	url := a.URL + resource

	reader, err := a.handleRequestBody(body, &options)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", url, reader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header = options.header

	resp, err := a.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer a.closeHttpBody(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, a.handleHttpError(resp)
	}
	return io.ReadAll(resp.Body)
}

func (a Api) Put(resource string, body any, opts ...HttpOption) ([]byte, error) {
	var options httpOptions
	options.apply(opts)
	url := a.URL + resource

	reader, err := a.handleRequestBody(body, &options)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("PUT", url, reader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header = options.header

	resp, err := a.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer a.closeHttpBody(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, a.handleHttpError(resp)
	}

	return io.ReadAll(resp.Body)
}

func (a Api) handleRequestBody(body any, options *httpOptions) (io.Reader, error) {
	if reader, ok := body.(io.Reader); ok {
		if _, ok = options.header["Content-Type"]; !ok {
			options.header.Set("Content-Type", "application/octet-stream")
		}
		return reader, nil
	} else {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reader := bytes.NewBuffer(jsonData) // no need to close
		if _, ok = options.header["Content-Type"]; !ok {
			options.header.Set("Content-Type", "application/json")
		}
		return reader, nil
	}
}

func (a Api) handleHttpError(resp *http.Response) error {
	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("API request failed with status %d and unreadable body", resp.StatusCode)
	}
	rid := resp.Header.Get("X-Request-ID")
	return fmt.Errorf("API request (id=%s) failed with status %d: %s", rid, resp.StatusCode, string(buf))
}

func (a Api) closeHttpBody(body io.Closer) {
	if err := body.Close(); err != nil {
		fmt.Printf("warning: failed to close response body: %v\n", err)
	}
}

type httpOptions struct {
	header http.Header
}

func (o *httpOptions) apply(opts []HttpOption) {
	if o.header == nil {
		o.header = make(http.Header)
	}

	for _, f := range opts {
		f(o)
	}
}
