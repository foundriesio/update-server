// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package auth

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/foundriesio/update-server/server"
	"github.com/foundriesio/update-server/storage"
	"github.com/labstack/echo/v4"
	"golang.org/x/time/rate"
)

var errTooManyRequests = errors.New("rate-limit exceeded")
var errTooManyBadAuthOps = errors.New("too many bad authentication operations")

// badAuthRate is the token-bucket refill rate for bad-auth tracking,
// set to 1 token per 60 seconds (i.e. 1 per minute).
const badAuthRate = rate.Limit(1.0 / 60)

func NewRateLimiter(cfg storage.RateLimitConfig) *authRateLimiter {
	if cfg.AttemptsPerSecond <= 0 {
		cfg.AttemptsPerSecond = 2
	}
	if cfg.AttemptsBlockDurationSec <= 0 {
		cfg.AttemptsBlockDurationSec = 30
	}
	if cfg.BadAuthLimit <= 0 {
		cfg.BadAuthLimit = 5
	}
	if cfg.BadAuthBlockDurationSec <= 0 {
		cfg.BadAuthBlockDurationSec = 300
	}

	slog.Info("RateLimiter initialized",
		"attemptsPerSecond", cfg.AttemptsPerSecond,
		"attemptsBlockDurationSec", cfg.AttemptsBlockDurationSec,
		"badAuthLimit", cfg.BadAuthLimit,
		"badAuthBlockDurationSec", cfg.BadAuthBlockDurationSec,
	)

	sweepAge := 2 * time.Duration(cfg.BadAuthBlockDurationSec) * time.Second
	if alt := 2 * time.Duration(cfg.AttemptsBlockDurationSec) * time.Second; alt > sweepAge {
		sweepAge = alt
	}

	rl := &authRateLimiter{
		attemptsPerSecond:     cfg.AttemptsPerSecond,
		attemptsBlockDuration: time.Duration(cfg.AttemptsBlockDurationSec) * time.Second,
		badAuthLimit:          cfg.BadAuthLimit,
		badAuthBlockDuration:  time.Duration(cfg.BadAuthBlockDurationSec) * time.Second,
		entries:               newGenerationMap[*ipLimiter](sweepAge),
	}
	rl.Middleware = func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			identifier := c.RealIP()
			if err := rl.allow(identifier); err != nil {
				return server.EchoError(c, err, http.StatusTooManyRequests, err.Error())
			}
			return next(c)
		}
	}

	return rl
}

// This implementation is similar to the echo rate limiter but done in a way that
// allows us to block IPs that have too many bad auth operations for a given amount of time
type authRateLimiter struct {
	Middleware echo.MiddlewareFunc

	mutex sync.Mutex

	entries generationMap[*ipLimiter]

	attemptsPerSecond     int
	attemptsBlockDuration time.Duration
	badAuthLimit          int
	badAuthBlockDuration  time.Duration
}

type ipLimiter struct {
	rateLimit   *rate.Limiter
	authLimit   *rate.Limiter
	blockUntil  time.Time
	blockReason error
}

func (rl *authRateLimiter) getOrCreate(identifier string) *ipLimiter {
	v, exists := rl.entries.get(identifier)
	if !exists {
		v = &ipLimiter{
			rateLimit: rate.NewLimiter(rate.Limit(rl.attemptsPerSecond), rl.attemptsPerSecond),
			authLimit: rate.NewLimiter(badAuthRate, rl.badAuthLimit),
		}
		rl.entries.put(identifier, v)
	}
	return v
}

func (rl *authRateLimiter) FlagBadOperation(c echo.Context) {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	identifier := c.RealIP()
	v := rl.getOrCreate(identifier)
	now := time.Now()
	if !v.authLimit.AllowN(now, 1) {
		v.blockUntil = now.Add(rl.badAuthBlockDuration)
		isoTime := v.blockUntil.UTC().Format(time.RFC3339)
		v.blockReason = fmt.Errorf("%w: You are blocked until %s", errTooManyBadAuthOps, isoTime)
	}
}

func (rl *authRateLimiter) allow(identifier string) error {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()
	rl.entries.sweep(now)

	v := rl.getOrCreate(identifier)

	// Check if this IP is already blocked due to bad auth operations
	if now.Before(v.blockUntil) || v.authLimit.Tokens() <= 0 {
		return v.blockReason
	}

	// Check the per-second rate limit; if exceeded, block the IP
	if !v.rateLimit.Allow() {
		v.blockUntil = now.Add(rl.attemptsBlockDuration)
		isoTime := v.blockUntil.UTC().Format(time.RFC3339)
		v.blockReason = fmt.Errorf("%w: You are blocked until %s", errTooManyRequests, isoTime)
		return v.blockReason
	}

	return nil
}
