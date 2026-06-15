// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package auth

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/foundriesio/update-server/context"
	"github.com/foundriesio/update-server/storage"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func testGet(t *testing.T, rl *authRateLimiter, flagBad bool) *httptest.ResponseRecorder {
	e := echo.New()
	e.HideBanner = true

	handler := func(c echo.Context) error {
		if flagBad {
			rl.FlagBadOperation(c)
		}
		return c.String(http.StatusOK, "ok")
	}
	e.GET("/test", handler)

	// First request should succeed
	ctx := context.CtxWithLog(t.Context(), slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req.WithContext(ctx), rec)
	err := rl.Middleware(handler)(c)
	if err != nil {
		// echo middleware errors require some hackery to set the correct status code and response body
		if httpErr, ok := err.(*echo.HTTPError); ok {
			rec.Code = httpErr.Code
			fmt.Fprintf(rec.Body, "%v", httpErr.Message)
		} else {
			t.Fatal("Unexpected middleware error", err)
		}
	}

	return rec
}

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(storage.RateLimitConfig{
		AttemptsPerSecond:        2,
		AttemptsBlockDurationSec: 1,
		BadAuthLimit:             2,
		BadAuthBlockDurationSec:  10,
	})

	// First two requests should succeed
	rec := testGet(t, rl, false)
	assert.Equal(t, http.StatusOK, rec.Code)
	rec = testGet(t, rl, false)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Third request will trigger rate limit
	rec = testGet(t, rl, false)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Contains(t, rec.Body.String(), errTooManyRequests.Error())

	// Test bad auth operations
	// Sleep 1.01 second for the rate limit to reset
	time.Sleep(1010 * time.Millisecond)
	rec = testGet(t, rl, true)
	assert.Equal(t, http.StatusOK, rec.Code)
	rec = testGet(t, rl, true)
	assert.Equal(t, http.StatusOK, rec.Code)

	time.Sleep(1010 * time.Millisecond) // don't exceed the ratelimit
	rec = testGet(t, rl, true)
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = testGet(t, rl, false)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Contains(t, rec.Body.String(), errTooManyBadAuthOps.Error())
}
