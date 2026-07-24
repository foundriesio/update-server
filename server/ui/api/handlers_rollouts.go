// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	storage "github.com/foundriesio/update-server/storage/api"
)

type Rollout = storage.Rollout
type Update = storage.Update
type UpdateReport = storage.UpdateReport

// @Summary List updates
// @Description Requires scope: updates:read or updates:read-update
// @Tags    Updates
// @Produce json
// @Success 200 {object} map[string][]Update
// @Param   tag path string true "Update tag"
// @Router  /updates/{tag} [get]
func (h *handlers) updateList(c echo.Context) error {
	tag := c.Param("tag")

	if updates, err := h.storage.ListUpdates(tag); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to look up updates")
	} else {
		if updates == nil {
			updates = map[string][]Update{}
		}
		return c.JSON(http.StatusOK, updates)
	}
}

// @Summary Get summary of update
// @Description Requires scope: updates:read or updates:read-update
// @Tags    Updates
// @Produce json
// @Success 200 {object} UpdateReport
// @Param   tag path string true "Update tag"
// @Param   update path string true "Update name"
// @Router  /updates/{tag}/{update}/report [get]
func (h *handlers) updateReport(c echo.Context) error {
	tag := c.Param("tag")
	updateName := c.Param("update")
	report, err := h.storage.UpdateReport(tag, updateName)
	if err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to get update report")
	}
	return c.JSON(http.StatusOK, report)
}

// @Summary Get summary of update
// @Description Requires scope: updates:read or updates:read-update
// @Tags    Updates
// @Produce json
// @Success 200 {object} UpdateReport
// @Param   tag path string true "Update tag"
// @Param   update path string true "Update name"
// @Param   rollout path string true "Rollout name"
// @Router  /updates/{tag}/{update}/rollouts/{rollout}/report [get]
func (h *handlers) updateRolloutReport(c echo.Context) error {
	tag := c.Param("tag")
	updateName := c.Param("update")
	rolloutName := c.Param("rollout")
	report, err := h.storage.RolloutReport(tag, updateName, rolloutName)
	if err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to get rollout report")
	}
	return c.JSON(http.StatusOK, report)
}

// @Summary Tail update logs
// @Description Requires scope: updates:read or updates:read-update
// @Tags    Updates
// @Produce text/plain
// @Success 200
// @Param   tag path string true "Update tag"
// @Param   update path string true "Update name"
// @Router  /updates/{tag}/{update}/tail [get]
func (h *handlers) updateTail(c echo.Context) error {
	ctx := c.Request().Context()
	tag := c.Param("tag")
	updateName := c.Param("update")
	// Read file infinitely until client disconnects (writes to ctx.Done() channel).
	reader := h.storage.TailRolloutsLog(tag, updateName, ctx.Done())
	return streamUpdateLogs(c, reader)
}

// @Summary List update rollouts
// @Description Requires scope: updates:read or updates:read-update
// @Tags    Updates
// @Produce json
// @Success 200 {array} string
// @Param   tag path string true "Update tag"
// @Param   update path string true "Update name"
// @Router  /updates/{tag}/{update}/rollouts [get]
func (h *handlers) rolloutList(c echo.Context) error {
	tag := c.Param("tag")
	updateName := c.Param("update")

	if rollouts, err := h.storage.ListRollouts(tag, updateName); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to look up update rollouts")
	} else {
		if rollouts == nil {
			rollouts = []string{}
		}
		return c.JSON(http.StatusOK, rollouts)
	}
}

// @Summary Get update rollout
// @Description Requires scope: updates:read or updates:read-update
// @Tags    Updates
// @Produce json
// @Success 200 {object} Rollout
// @Param   tag path string true "Update tag"
// @Param   update path string true "Update name"
// @Param   rollout path string true "Rollout name"
// @Router  /updates/{tag}/{update}/rollouts/{rollout} [get]
func (h *handlers) rolloutGet(c echo.Context) error {
	tag := c.Param("tag")
	updateName := c.Param("update")
	rolloutName := c.Param("rollout")

	if rollout, err := h.storage.GetRollout(tag, updateName, rolloutName); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return EchoError(c, err, http.StatusNotFound, "Not found rollout")
		} else {
			return EchoError(c, err, http.StatusInternalServerError, "Failed to look up update rollout")
		}
	} else {
		return c.JSON(http.StatusOK, rollout)
	}
}

// @Summary Create update rollout
// @Description Requires scope: updates:read-update
// @Tags    Updates
// @Accept json
// @Param data body Rollout true "Rollout data"
// @Produce json
// @Success 202
// @Param   tag path string true "Update tag"
// @Param   update path string true "Update name"
// @Param   rollout path string true "Rollout name"
// @Router  /updates/{tag}/{update}/rollouts/{rollout} [put]
func (h *handlers) rolloutPut(c echo.Context) error {
	ctx := c.Request().Context()
	tag := c.Param("tag")
	updateName := c.Param("update")
	rolloutName := c.Param("rollout")
	var (
		rollout Rollout
		err     error
	)
	if err = c.Bind(&rollout); err != nil {
		return EchoError(c, err, http.StatusBadRequest, "Bad JSON body")
	}
	if len(rollout.Uuids) == 0 && len(rollout.Groups) == 0 {
		return c.String(http.StatusBadRequest, "Either uuids or groups must be set")
	}
	if len(rollout.Effect) > 0 {
		return c.String(http.StatusBadRequest, "Effective uuids are readonly")
	}

	// Check if update with this name exists
	if updates, err := h.storage.ListUpdates(tag); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to check if update exists")
	} else if tagUpdates, ok := updates[tag]; !ok || !slices.ContainsFunc(tagUpdates, func(u storage.Update) bool {
		return u.Name == updateName
	}) {
		return c.String(http.StatusNotFound, "Update with this name does not exist")
	}

	// Check if rollout with this name already exists
	if _, err = h.storage.GetRollout(tag, updateName, rolloutName); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return EchoError(c, err, http.StatusInternalServerError, "Failed to check if rollout exists")
		}
	} else {
		return c.String(http.StatusConflict, "Rollout with this name already exists")
	}

	if err = h.storage.CreateRollout(tag, updateName, rolloutName, rollout); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to save rollout to disk")
	}
	go func() {
		if err := h.storage.CommitRollout(tag, updateName, rolloutName, rollout); err != nil {
			// Background daemon should correct any database inconsistency, so we still return success here.
			CtxGetLog(ctx).Error("Failed to update devices for rollout", "error", err)
		}
	}()
	return c.NoContent(http.StatusAccepted)
}

// @Summary Tail rollout logs
// @Description Requires scope: updates:read or updates:read-update
// @Tags    Updates
// @Produce text/plain
// @Success 200
// @Param   tag path string true "Update tag"
// @Param   update path string true "Update name"
// @Param   rollout path string true "Rollout name"
// @Router  /updates/{tag}/{update}/rollouts/{rollout}/tail [get]
func (h *handlers) rolloutTail(c echo.Context) error {
	ctx := c.Request().Context()
	tag := c.Param("tag")
	updateName := c.Param("update")
	rolloutName := c.Param("rollout")
	if rollout, err := h.storage.GetRollout(tag, updateName, rolloutName); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return EchoError(c, err, http.StatusNotFound, "Not found rollout")
		} else {
			return EchoError(c, err, http.StatusInternalServerError, "Failed to look up update rollout")
		}
	} else if !rollout.Commit {
		// Notify the client to retry later with a single error event.
		reader := func(yield func(string, error) bool) {
			yield("", errors.New("Rollout was not yet committed"))
		}
		return streamUpdateLogs(c, reader)
	} else {
		// Read file infinitely until client disconnects (writes to ctx.Done() channel).
		reader := h.storage.TailRolloutsLog(tag, updateName, ctx.Done())
		reader = filterUpdateLogs(rollout.Effect, reader)
		return streamUpdateLogs(c, reader)
	}
}

func validateUpdateParams(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if tag := c.Param("tag"); len(tag) > 0 && !validateTag(tag) {
			return echo.NewHTTPError(http.StatusNotFound, "Tag must match a given regexp: "+validTagRegex)
		} else if update := c.Param("update"); len(update) > 0 && !validateUpdate(update) {
			return echo.NewHTTPError(http.StatusNotFound, "Update name must match a given regexp: "+validUpdateRegex)
		} else if rollout := c.Param("rollout"); len(rollout) > 0 && !validateRollout(rollout) {
			return echo.NewHTTPError(http.StatusNotFound, "Rollout name must match a given regexp: "+validRolloutRegex)
		}
		return next(c)
	}
}

const (
	validTagRegex     = `^[a-zA-Z0-9_\-\.\+]+$`
	validUpdateRegex  = `^[a-zA-Z0-9_\-\.]+$`
	validRolloutRegex = validUpdateRegex
)

var (
	validateTag     = regexp.MustCompile(validTagRegex).MatchString
	validateUpdate  = regexp.MustCompile(validUpdateRegex).MatchString
	validateRollout = regexp.MustCompile(validRolloutRegex).MatchString
)

func parseLastEventId(c echo.Context) int {
	r := c.Request()
	val := r.Header.Get("Last-Event-ID")
	if len(val) == 0 {
		return 0
	} else if res, err := strconv.Atoi(val); err != nil {
		CtxGetLog(r.Context()).Warn("Invalid Last-Event-ID - ignoring", "value", val)
		return 0
	} else {
		return res
	}
}

func streamUpdateLogs(c echo.Context, reader iter.Seq2[string, error]) error {
	log := CtxGetLog(c.Request().Context())
	lastId := parseLastEventId(c)
	r := c.Response()
	r.Header().Set("Content-Type", "text/event-stream")
	// Below two headers prevent proxy caching and buffering.
	r.Header().Set("Cache-Control", "no-cache")
	r.Header().Set("X-Accel-Buffering", "no")

	eventStreamReader := func(yield func(string, error) bool) {
		index := 0
		for line, err := range reader {
			if err != nil {
				// Preserve the same event ID as the last success, so that client resumes at the correct line.
				// If there was no success yet - index is zero, meaning restart from the beginning.
				msg := fmt.Sprintf("event: error\nid: %d\nretry: 1000\n", index)
				if errors.Is(err, os.ErrNotExist) {
					msg += "data: No rollout logs for this update yet.\n\n"
				} else {
					log.Error("Failed to tail logs", "error", err)
					msg += "data: Logs tail was interrupted due to server error.\n\n"
				}
				_ = yield(msg, nil)
				break
			}
			if index += 1; index <= lastId {
				continue
			}
			line = fmt.Sprintf("event: log\nid: %d\ndata: %s\n\n", index, line)
			if !yield(line, nil) {
				break
			}
		}
	}

	// Errors are already handled by the eventStreamReader
	for line := range keepaliveReader(eventStreamReader) {
		if _, err := r.Write([]byte(line)); err != nil {
			// Client disconnected - only log unexpected errors
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				log.Error("Failed to write log tail to client", "error", err)
			}
			break
		}
		r.Flush()
	}
	return nil
}

func filterUpdateLogs(uuids []string, reader iter.Seq2[string, error]) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		for line, err := range reader {
			if err == nil {
				var status storage.DeviceStatus
				if err = json.Unmarshal([]byte(line), &status); err == nil {
					if !slices.Contains(uuids, status.Uuid) {
						continue
					}
				}
			}
			if !yield(line, err) {
				break
			}
		}
	}
}

var (
	keepaliveResponseText     = ": idle\n\n"
	keepaliveResponseInterval = 30 * time.Second
)

func keepaliveReader(reader iter.Seq2[string, error]) iter.Seq2[string, error] {
	// An HTTP client will disconnect after an idle time while server does not write annything (usually 5 minutes).
	// So, in order to keep alive the tail connection, send a comment event, ignored by browser event handlers.
	return func(yield func(string, error) bool) {
		type lineStruct struct {
			line string
			err  error
		}
		lineChan := make(chan lineStruct)
		done := make(chan bool)
		go func() {
		READ:
			for line, err := range reader {
				// Non-blocking read to check if keepalive polling was stopped.
				select {
				case <-done:
					break READ
				default:
					// No signal to stop => emit a new line.
					lineChan <- lineStruct{line, err}
				}
			}
			close(lineChan)
		}()
	LOOP:
		for {
			select {
			case lineWrap, ok := <-lineChan:
				if !ok {
					// Reader finished its loop.
					break LOOP
				} else if !yield(lineWrap.line, lineWrap.err) {
					// Caller signals to stop reading.
					break LOOP
				}
			case <-time.After(keepaliveResponseInterval):
				if !yield(keepaliveResponseText, nil) {
					break LOOP
				}
			}
		}
		// Non-blocking write to signal that keepalive polling stops.
		// A file reading thread might have already finished by this time.
		select {
		case done <- true:
		default:
		}
		close(done)
	}
}
