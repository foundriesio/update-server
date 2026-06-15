// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package server

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/random"

	"github.com/foundriesio/update-server/context"
)

func NewEchoServer() *echo.Echo {
	server := echo.New()
	server.HideBanner = true
	server.HidePort = true
	server.Use(contextLogger())
	server.Use(middlewareLogger())
	return server
}

func EchoError(c echo.Context, err error, code int, msg string) error {
	log := context.CtxGetLog(c.Request().Context())
	log.Error(msg, "error", err)
	// Not c.String(code, msg), as it would return nil
	return echo.NewHTTPError(code, msg)
}

func ReadBody(c echo.Context) ([]byte, error) {
	req := c.Request()
	if bytes, err := io.ReadAll(req.Body); err != nil {
		return nil, EchoError(c, err, http.StatusBadRequest, "Failed to read request body")
	} else {
		return bytes, nil
	}
}

func ReadJsonBody(c echo.Context, res any) error {
	if bytes, err := ReadBody(c); err != nil {
		return err
	} else {
		return ParseJsonBody(c, bytes, &res)
	}
}

func ParseJsonBody(c echo.Context, bytes []byte, res any) error {
	if err := json.Unmarshal(bytes, &res); err != nil {
		return EchoError(c, err, http.StatusBadRequest, "Failed to parse request JSON body")
	} else {
		return nil
	}
}

func middlewareLogger() echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		HandleError:      true, // forwards error to the global error handler, so it can decide appropriate status code
		LogContentLength: true,
		LogError:         true,
		LogMethod:        true,
		LogStatus:        true,
		LogURI:           true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			log := context.CtxGetLog(c.Request().Context())
			args := []any{"method", v.Method, "content-length", v.ContentLength, "status", v.Status}
			if v.Error == nil {
				log.Info("response", args...)
			} else {
				args = append(args, "err", v.Error.Error())
				log.Error("response", args...)
			}
			return nil
		},
	})
}

func contextLogger() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			res := c.Response()
			ctx := req.Context()
			log := context.CtxGetLog(ctx)

			rid := req.Header.Get(echo.HeaderXRequestID)
			if rid == "" {
				rid = random.String(12) // No need for uuid, save some space
			}
			res.Header().Set(echo.HeaderXRequestID, rid)
			log = log.With("req_id", rid, "uri", req.RequestURI)
			ctx = context.CtxWithLog(ctx, log)
			c.SetRequest(req.WithContext(ctx))
			return next(c)
		}
	}
}
