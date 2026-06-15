// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package gateway

import (
	"net/http"
	"time"

	storage "github.com/foundriesio/update-server/storage/gateway"
	"github.com/labstack/echo/v4"
)

// @Summary Create update events
// @Accept  json
// @Param   events body []UpdateEvent true "Update events"
// @Produce plain
// @Success 200 ""
// @Router  /events [post]
func (handlers) eventsUpload(c echo.Context) error {
	ctx := c.Request().Context()
	log := CtxGetLog(ctx)
	d := CtxGetDevice(ctx)

	var events, validEvents []UpdateEvent
	if err := ReadJsonBody(c, &events); err != nil {
		return err
	}
	// Apply zero-error logic below this line:
	// As long as an upload has valid JSON structure, we should not return validation errors.

	for _, event := range events {
		if len(event.Id) == 0 {
			log.Warn("Missing event ID - skip it", "corr-id", event.Event.CorrelationId)
			continue
		}
		if len(event.Event.CorrelationId) == 0 {
			log.Warn("Missing event correlation ID - skip it", "event", event.Id)
			continue
		}
		if !storage.ValidCorrelationId(event.Event.CorrelationId) {
			log.Warn("Invalid event correlation ID - skip it", "event", event.Id, "corr-id", event.Event.CorrelationId)
			continue
		}
		if _, err := time.Parse(time.RFC3339, event.DeviceTime); err != nil {
			// The UI needs this field to be a valid datetime.  If it is not - warn and substitute it
			// with the current time.  Normally, the time skew should be within seconds.
			log.Warn("Invalid event deviceTime, must be RFC3339 - use current time",
				"error", err, "value", event.DeviceTime, "event", event.Id, "corr-id", event.Event.CorrelationId)
			event.DeviceTime = time.Now().UTC().Format(time.RFC3339)
		}
		validEvents = append(validEvents, event)
	}

	if len(validEvents) == 0 {
		// When device sent zero valid events, we should still succeed.
		return c.String(http.StatusOK, "")
	}
	if err := d.ProcessEvents(validEvents); err != nil {
		return EchoError(c, err, http.StatusInternalServerError, "Failed to save events")
	}
	return c.String(http.StatusOK, "")
}
