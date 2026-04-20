// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package api

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/foundriesio/dg-satellite/auth"
	"github.com/foundriesio/dg-satellite/context"
	"github.com/foundriesio/dg-satellite/server"
	"github.com/foundriesio/dg-satellite/server/ui/daemons"
	"github.com/foundriesio/dg-satellite/storage"
	apiStorage "github.com/foundriesio/dg-satellite/storage/api"
	gatewayStorage "github.com/foundriesio/dg-satellite/storage/gateway"
	storageTesting "github.com/foundriesio/dg-satellite/storage/testing"
	"github.com/foundriesio/dg-satellite/storage/users"
)

func generateUpdateEvents(corId, pack string, num int) []storage.DeviceUpdateEvent {
	events := make([]storage.DeviceUpdateEvent, num)
	for i := 0; i < num; i++ {
		events[i] = storage.DeviceUpdateEvent{
			Id:         fmt.Sprintf("%d_%s", i, corId),
			DeviceTime: "2023-12-12T12:00:00",
			Event: storage.DeviceEvent{
				CorrelationId: corId,
				Ecu:           "",
				Success:       nil,
				TargetName:    "intel-corei7-64-lmp-23",
				Version:       "23",
				Details:       pack,
			},
			EventType: storage.DeviceEventType{
				Id:      "EcuDownloadStarted",
				Version: 0,
			},
		}
	}
	return events
}

type testClient struct {
	t   *testing.T
	ctx Context
	fs  *apiStorage.FsHandle
	api *apiStorage.Storage
	gw  *gatewayStorage.Storage
	u   *users.User
	e   *echo.Echo
}

func (c testClient) Do(req *http.Request) *httptest.ResponseRecorder {
	req = req.WithContext(c.ctx)
	rec := httptest.NewRecorder()
	c.e.ServeHTTP(rec, req)
	return rec
}

func (c testClient) DoAsync(req *http.Request, done chan<- bool) *httptest.ResponseRecorder {
	req = req.WithContext(c.ctx)
	rec := httptest.NewRecorder()
	go func() {
		c.e.ServeHTTP(rec, req)
		if done != nil {
			done <- true
			close(done)
		}
	}()
	return rec
}

func (c testClient) assertDone(done <-chan bool) {
	select {
	case <-done:
		break
	default:
		require.Fail(c.t, "Must be done")
	}
}

func (c testClient) assertNotDone(done <-chan bool) {
	select {
	case <-done:
		require.Fail(c.t, "Must be not done")
	default:
		break
	}
}

func (c testClient) GET(resource string, status int, headers ...string) []byte {
	req := httptest.NewRequest(http.MethodGet, "/v1"+resource, nil)
	c.marshalHeaders(headers, req)
	rec := c.Do(req)
	require.Equal(c.t, status, rec.Code)
	return rec.Body.Bytes()
}

func (c testClient) DELETE(resource string, status int, headers ...string) []byte {
	req := httptest.NewRequest(http.MethodDelete, "/v1"+resource, nil)
	c.marshalHeaders(headers, req)
	rec := c.Do(req)
	require.Equal(c.t, status, rec.Code)
	return rec.Body.Bytes()
}
func (c testClient) PATCH(resource string, status int, data any, headers ...string) []byte {
	req := httptest.NewRequest(http.MethodPatch, "/v1"+resource, c.marshalBody(data))
	c.marshalHeaders(headers, req)
	rec := c.Do(req)
	require.Equal(c.t, status, rec.Code)
	return rec.Body.Bytes()
}

func (c testClient) POST(resource string, status int, body io.Reader, headers ...string) []byte {
	req := httptest.NewRequest(http.MethodPost, "/v1"+resource, body)
	c.marshalHeaders(headers, req)
	rec := c.Do(req)
	require.Equal(c.t, status, rec.Code)
	return rec.Body.Bytes()
}

func (c testClient) PUT(resource string, status int, data any, headers ...string) []byte {
	req := httptest.NewRequest(http.MethodPut, "/v1"+resource, c.marshalBody(data))
	c.marshalHeaders(headers, req)
	rec := c.Do(req)
	require.Equal(c.t, status, rec.Code)
	return rec.Body.Bytes()
}

func (c testClient) marshalHeaders(headers []string, req *http.Request) {
	require.Zero(c.t, len(headers)%2, "Headers must be a sequence of names and values - even number")
	req.Header.Set("Authorization", "unit-test") // needed for csrfCheck middleware to work in tests
	for i := 0; i < len(headers)/2; i++ {
		req.Header.Add(headers[i*2], headers[i*2+1])
	}
}

func (c testClient) marshalBody(data any) io.Reader {
	if s, ok := data.(string); ok {
		return strings.NewReader(s)
	} else if b, ok := data.([]byte); ok {
		return bytes.NewReader(b)
	} else if r, ok := data.(io.Reader); ok {
		return r
	} else {
		b, err := json.Marshal(data)
		require.Nil(c.t, err)
		return bytes.NewReader(b)
	}
}

type testAuthProvider struct {
	user *users.User
}

func (testAuthProvider) Name() string {
	return "test"
}

func (p testAuthProvider) Configure(e *echo.Echo, userS *users.Storage, config *storage.AuthConfig) error {
	return nil
}

func (p testAuthProvider) GetUser(c echo.Context) (*users.User, error) {
	return p.user, nil
}

func (p testAuthProvider) GetSession(c echo.Context) (*auth.Session, error) {
	return &auth.Session{}, nil
}

func (testAuthProvider) DropSession(echo.Context, *auth.Session) {
}

func NewTestClient(t *testing.T) *testClient {
	ctx := context.Background()
	tmpDir := t.TempDir()
	fsS, err := apiStorage.NewFs(tmpDir)
	require.Nil(t, err)
	db, err := apiStorage.NewDb(filepath.Join(tmpDir, apiStorage.DbFile))
	require.Nil(t, err)
	apiS, err := apiStorage.NewStorage(db, fsS)
	require.Nil(t, err)
	gwS, err := gatewayStorage.NewStorage(db, fsS)
	require.Nil(t, err)

	log, err := context.InitLogger("debug")
	require.Nil(t, err)
	ctx = CtxWithLog(ctx, log)

	e := server.NewEchoServer()
	require.Nil(t, err)
	u := &users.User{
		Username:      "root",
		AllowedScopes: 0,
	}
	RegisterHandlers(e, apiS, &testAuthProvider{user: u})

	tc := testClient{
		t:   t,
		ctx: ctx,
		fs:  fsS,
		api: apiS,
		gw:  gwS,
		u:   u,
		e:   e,
	}
	return &tc
}

func TestApiDeviceList(t *testing.T) {
	tc := NewTestClient(t)
	tc.GET("/devices", 403)
	tc.u.AllowedScopes = users.ScopeDevicesR

	// No devices
	data := tc.GET("/devices", 200)
	var devices []apiStorage.DeviceListItem
	require.Nil(t, json.Unmarshal(data, &devices))
	require.Len(t, devices, 0)

	// two devices with different last seen times
	_, err := tc.gw.DeviceCreate("test-device-1", "pubkey1", true)
	require.Nil(t, err)
	time.Sleep(1 * time.Second)
	_, err = tc.gw.DeviceCreate("test-device-2", "pubkey2", false)
	require.Nil(t, err)

	data = tc.GET("/devices", 200)
	require.Nil(t, json.Unmarshal(data, &devices))
	require.Len(t, devices, 2)
	// default sort is name-asc (name, uuid)
	assert.Equal(t, "test-device-1", devices[0].Uuid)
	assert.Equal(t, "test-device-2", devices[1].Uuid)

	// test sorting
	data = tc.GET("/devices?order-by=last-seen-asc", 200)
	require.Nil(t, json.Unmarshal(data, &devices))
	assert.Equal(t, "test-device-1", devices[0].Uuid)
	assert.Equal(t, "test-device-2", devices[1].Uuid)
	data = tc.GET("/devices?order-by=last-seen-desc", 200)
	require.Nil(t, json.Unmarshal(data, &devices))
	assert.Equal(t, "test-device-2", devices[0].Uuid)
	assert.Equal(t, "test-device-1", devices[1].Uuid)
	data = tc.GET("/devices?order-by=created-at-desc", 200)
	require.Nil(t, json.Unmarshal(data, &devices))
	assert.Equal(t, "test-device-2", devices[0].Uuid)
	assert.Equal(t, "test-device-1", devices[1].Uuid)
	data = tc.GET("/devices?order-by=name-desc", 200)
	require.Nil(t, json.Unmarshal(data, &devices))
	assert.Equal(t, "test-device-2", devices[0].Uuid)
	assert.Equal(t, "test-device-1", devices[1].Uuid)
	data = tc.GET("/devices?order-by=uuid-desc", 200)
	require.Nil(t, json.Unmarshal(data, &devices))
	assert.Equal(t, "test-device-2", devices[0].Uuid)
	assert.Equal(t, "test-device-1", devices[1].Uuid)

	// test pagination Link headers
	req := httptest.NewRequest(http.MethodGet, "/v1/devices?limit=1&order-by=name-asc", nil)
	rec := tc.Do(req)
	require.Equal(t, 200, rec.Code)
	require.Nil(t, json.Unmarshal(rec.Body.Bytes(), &devices))
	require.Len(t, devices, 1)
	linkHeader := rec.Header().Get("Link")
	assert.Contains(t, linkHeader, `rel="first"`)
	assert.Contains(t, linkHeader, `rel="next"`)
	assert.Contains(t, linkHeader, `rel="last"`)
	assert.Contains(t, linkHeader, "offset=1")

	// last page — no "next" link
	req = httptest.NewRequest(http.MethodGet, "/v1/devices?limit=1&offset=1&order-by=name-asc", nil)
	rec = tc.Do(req)
	require.Equal(t, 200, rec.Code)
	require.Nil(t, json.Unmarshal(rec.Body.Bytes(), &devices))
	require.Len(t, devices, 1)
	linkHeader = rec.Header().Get("Link")
	assert.Contains(t, linkHeader, `rel="first"`)
	assert.NotContains(t, linkHeader, `rel="next"`)
	assert.Contains(t, linkHeader, `rel="last"`)

	// Set device name to override the uuid sort.
	tc.u.AllowedScopes = users.ScopeDevicesRU
	tc.PATCH("/devices/test-device-2/labels", 200,
		`{"upserts":{"name":"test-device-3"}}`, "content-type", "application/json")
	// Device with name before device without name.
	data = tc.GET("/devices?order-by=name-asc", 200)
	require.Nil(t, json.Unmarshal(data, &devices))
	assert.Equal(t, "test-device-2", devices[0].Uuid)
	assert.Equal(t, "test-device-1", devices[1].Uuid)
	data = tc.GET("/devices?order-by=name-desc", 200)
	require.Nil(t, json.Unmarshal(data, &devices))
	assert.Equal(t, "test-device-2", devices[0].Uuid)
	assert.Equal(t, "test-device-1", devices[1].Uuid)
	// Order by UUID is not affected
	data = tc.GET("/devices?order-by=uuid-asc", 200)
	require.Nil(t, json.Unmarshal(data, &devices))
	assert.Equal(t, "test-device-1", devices[0].Uuid)
	assert.Equal(t, "test-device-2", devices[1].Uuid)

	tc.PATCH("/devices/test-device-1/labels", 200,
		`{"upserts":{"name":"test-device-1"}}`, "content-type", "application/json")
	// Both devices have a name - order restored.
	data = tc.GET("/devices?order-by=name-asc", 200)
	require.Nil(t, json.Unmarshal(data, &devices))
	assert.Equal(t, "test-device-1", devices[0].Uuid)
	assert.Equal(t, "test-device-2", devices[1].Uuid)
	data = tc.GET("/devices?order-by=name-desc", 200)
	require.Nil(t, json.Unmarshal(data, &devices))
	assert.Equal(t, "test-device-2", devices[0].Uuid)
	assert.Equal(t, "test-device-1", devices[1].Uuid)

}

func TestApiDeviceGet(t *testing.T) {
	tc := NewTestClient(t)
	tc.GET("/devices/foo", 403)
	tc.u.AllowedScopes = users.ScopeDevicesR

	_ = tc.GET("/devices/does-not-exist", 404)

	_, err := tc.gw.DeviceCreate("test-device-1", "pubkey1", true)
	require.Nil(t, err)
	_, err = tc.gw.DeviceCreate("test-device-2", "pubkey2", false)
	require.Nil(t, err)

	data := tc.GET("/devices/test-device-1", 200)
	var device apiStorage.Device
	require.Nil(t, json.Unmarshal(data, &device))
	assert.Equal(t, "test-device-1", device.Uuid)
	assert.Equal(t, "pubkey1", device.PubKey)

	data = tc.GET("/devices/test-device-2", 200)
	require.Nil(t, json.Unmarshal(data, &device))
	assert.Equal(t, "test-device-2", device.Uuid)
	assert.Equal(t, "pubkey2", device.PubKey)

	// Test sys-info files
	require.Nil(t, tc.fs.Devices.WriteFile("test-device-1", storage.AktomlFile, "test-aktoml"))
	require.Nil(t, tc.fs.Devices.WriteFile("test-device-1", storage.NetInfoFile, "netinfo"))
	require.Nil(t, tc.fs.Devices.WriteFile("test-device-1", storage.HwInfoFile, "lshw"))
	data = tc.GET("/devices/test-device-1", 200)
	require.Nil(t, json.Unmarshal(data, &device))
	require.Equal(t, "test-aktoml", device.Aktoml)
	require.Equal(t, "netinfo", device.NetInfo)
	require.Equal(t, "lshw", device.HwInfo)

}

func TestApiDeviceLabelsPatch(t *testing.T) {
	tc := NewTestClient(t)
	_, err := tc.gw.DeviceCreate("test-device-1", "pubkey1", true)
	require.Nil(t, err)
	_, err = tc.gw.DeviceCreate("test-device-2", "pubkey2", false)
	require.Nil(t, err)

	headers := []string{"content-type", "application/json"}
	data := `{"upserts":{"name":"test","foo":"bar"}}`
	tc.PATCH("/devices/test-device-1/labels", 403, data, headers...)
	tc.GET("/known-labels/devices", 403)
	tc.GET("/known-labels/device-groups", 403)
	tc.u.AllowedScopes = users.ScopeDevicesR
	tc.PATCH("/devices/test-device-1/labels", 403, data, headers...)

	var labels, groups []string
	require.Nil(t, json.Unmarshal(tc.GET("/known-labels/devices", 200), &labels))
	assert.Equal(t, []string{"name", "group"}, labels)
	require.Nil(t, json.Unmarshal(tc.GET("/known-labels/device-groups", 200), &groups))
	assert.Equal(t, []string{}, groups)

	tc.u.AllowedScopes = users.ScopeDevicesRU
	tc.PATCH("/devices/test-device-1/labels", 200, data, headers...)

	var device apiStorage.Device
	require.Nil(t, json.Unmarshal(tc.GET("/devices/test-device-1", 200), &device))
	assert.Equal(t, "test", device.Labels["name"])
	assert.Equal(t, "bar", device.Labels["foo"])

	require.Nil(t, json.Unmarshal(tc.GET("/known-labels/devices", 200), &labels))
	assert.Equal(t, []string{"name", "group", "foo"}, labels)

	data = `{"upserts":{"bar":"baz","group":"test"},"deletes":["foo"]}}`
	tc.PATCH("/devices/test-device-1/labels", 200, data, headers...)

	device = apiStorage.Device{}
	require.Nil(t, json.Unmarshal(tc.GET("/devices/test-device-1", 200), &device))
	assert.Equal(t, "test", device.Labels["name"])
	assert.Equal(t, "", device.Labels["foo"])
	assert.Equal(t, "baz", device.Labels["bar"])

	require.Nil(t, json.Unmarshal(tc.GET("/known-labels/devices", 200), &labels))
	assert.Equal(t, []string{"name", "group", "bar", "foo"}, labels)
	require.Nil(t, json.Unmarshal(tc.GET("/known-labels/device-groups", 200), &groups))
	assert.Equal(t, []string{"test"}, groups)

	data = `Bad JSON`
	tc.PATCH("/devices/test-device-1/labels", 400, data, headers...)
	data = `{"upserts":{"label-name-too-long-should-fail":"foo"}}`
	tc.PATCH("/devices/test-device-1/labels", 400, data, headers...)
	data = `{"upserts":{"foo":"label-value-too-long-should-also-fail-but-its-allowed-length-limit-is-much-much-higher"}}`
	tc.PATCH("/devices/test-device-1/labels", 400, data, headers...)
	data = `{"upserts":{"special+name":"baz"}}`
	tc.PATCH("/devices/test-device-1/labels", 400, data, headers...)
	data = `{"upserts":{"foo":"special&value"}}`
	tc.PATCH("/devices/test-device-1/labels", 400, data, headers...)

	// Duplicates are not allowed for a "name" label, but allowed for other labels.
	// Note that label names are lowercase only i.e. there can be a label "name" but not "Name".
	data = `{"upserts":{"name":"test"}}`
	tc.PATCH("/devices/test-device-2/labels", 409, data, headers...)
	data = `{"upserts":{"name":"test2","bar":"baz","group":"test"}}`
	tc.PATCH("/devices/test-device-2/labels", 200, data, headers...)
	data = `{"upserts":{"name":"test-2","group":"other"}}`
	tc.PATCH("/devices/test-device-2/labels", 200, data, headers...)

	require.Nil(t, json.Unmarshal(tc.GET("/known-labels/devices", 200), &labels))
	assert.Equal(t, []string{"name", "group", "bar", "foo"}, labels)
	require.Nil(t, json.Unmarshal(tc.GET("/known-labels/device-groups", 200), &groups))
	assert.Equal(t, []string{"other", "test"}, groups)

	// Unlike label names which are remembered forever, group names are forgotten when no device belong to them.
	data = `{"upserts":{"group":"new"}}`
	tc.PATCH("/devices/test-device-2/labels", 200, data, headers...)
	require.Nil(t, json.Unmarshal(tc.GET("/known-labels/device-groups", 200), &groups))
	assert.Equal(t, []string{"new", "test"}, groups)

	// Group names can also go from the group configs stored in the file system.
	// These are always returned, even if the underlying config was effectively zeroed, as we still keep config history.
	require.Nil(t, tc.fs.Configs.WriteGroupConfig("test", "anything"))
	require.Nil(t, tc.fs.Configs.WriteGroupConfig("cfg", "anything"))
	require.Nil(t, tc.fs.Configs.WriteGroupConfig("ok", ""))
	require.Nil(t, json.Unmarshal(tc.GET("/known-labels/device-groups", 200), &groups))
	assert.Equal(t, []string{"cfg", "new", "ok", "test"}, groups)
}

func TestApiDeviceLabelsPut(t *testing.T) {
	tc := NewTestClient(t)
	_, err := tc.gw.DeviceCreate("test-device-1", "pubkey1", true)
	require.Nil(t, err)
	_, err = tc.gw.DeviceCreate("test-device-2", "pubkey2", false)
	require.Nil(t, err)

	headers := []string{"content-type", "application/json"}
	data := `{"foo":"bar", "name":"test"}`
	tc.PUT("/devices/test-device-1/labels", 403, data, headers...)
	tc.u.AllowedScopes = users.ScopeDevicesR
	tc.PUT("/devices/test-device-1/labels", 403, data, headers...)
	tc.u.AllowedScopes = users.ScopeDevicesRU
	tc.PUT("/devices/test-device-1/labels", 200, data, headers...)

	var device apiStorage.Device
	require.Nil(t, json.Unmarshal(tc.GET("/devices/test-device-1", 200), &device))
	assert.Equal(t, "bar", device.Labels["foo"])
	assert.Equal(t, "test", device.Labels["name"])

	// Duplicates are not allowed for a "name" label, but allowed for other labels.
	tc.PUT("/devices/test-device-2/labels", 409, data, headers...)
	data = `{"foo":"bar"}`
	tc.PUT("/devices/test-device-2/labels", 200, data, headers...)
	device = apiStorage.Device{}
	require.Nil(t, json.Unmarshal(tc.GET("/devices/test-device-2", 200), &device))
	assert.Equal(t, "bar", device.Labels["foo"])
	assert.Equal(t, "", device.Labels["name"])

	data = `{"name":"test2"}`
	tc.PUT("/devices/test-device-2/labels", 200, data, headers...)
	device = apiStorage.Device{}
	require.Nil(t, json.Unmarshal(tc.GET("/devices/test-device-2", 200), &device))
	assert.Equal(t, "", device.Labels["foo"])
	assert.Equal(t, "test2", device.Labels["name"])
}

func TestApiAppsStates(t *testing.T) {
	tc := NewTestClient(t)
	tc.GET("/devices/test-device-1/apps-states", 403)
	tc.u.AllowedScopes = users.ScopeDevicesR

	_ = tc.GET("/devices/test-device-1/apps-states", 404)

	d, err := tc.gw.DeviceCreate("test-device-1", "pubkey1", true)
	require.Nil(t, err)

	state1 := storage.AppsStates{
		DeviceTime: "1",
	}
	stateBytes, err := json.Marshal(state1)
	require.Nil(t, err)
	require.Nil(t, d.SaveAppsStates(string(stateBytes)))

	state2 := storage.AppsStates{
		DeviceTime: "2",
	}
	stateBytes, err = json.Marshal(state2)
	require.Nil(t, err)
	require.Nil(t, d.SaveAppsStates(string(stateBytes)))

	res := tc.GET("/devices/test-device-1/apps-states", 200)
	var statesResp AppsStatesResp
	require.Nil(t, json.Unmarshal(res, &statesResp))
	require.Len(t, statesResp.AppsStates, 2)

	require.Equal(t, "1", statesResp.AppsStates[1].DeviceTime)
	require.Equal(t, "2", statesResp.AppsStates[0].DeviceTime)
}

func TestApiDeviceUpdateEvents(t *testing.T) {
	tc := NewTestClient(t)
	tc.GET("/devices/foo/updates", 403)
	tc.u.AllowedScopes = users.ScopeDevicesR

	_ = tc.GET("/devices/updates/does-not-exist", 404)

	d, err := tc.gw.DeviceCreate("test-device-1", "pubkey1", true)
	require.Nil(t, err)

	data := tc.GET("/devices/test-device-1/updates", 200)
	var updates []string
	require.Nil(t, json.Unmarshal(data, &updates))
	require.Len(t, updates, 0)

	events := generateUpdateEvents("uuid-1", "first", 2)
	require.Nil(t, d.ProcessEvents(events))
	events = generateUpdateEvents("uuid-2", "second", 3)
	require.Nil(t, d.ProcessEvents(events))

	data = tc.GET("/devices/test-device-1/updates", 200)
	require.Nil(t, json.Unmarshal(data, &updates))
	require.Len(t, updates, 2)

	data = tc.GET("/devices/test-device-1/updates/"+updates[0], 200)
	require.Nil(t, json.Unmarshal(data, &events))
	assert.Equal(t, "second", events[0].Event.Details)

	data = tc.GET("/devices/test-device-1/updates/"+updates[1], 200)
	require.Nil(t, json.Unmarshal(data, &events))
	assert.Equal(t, "first", events[1].Event.Details)

	_ = tc.GET("/devices/test-device-1/updates/doesnoexist", 404)
}

func TestApiUpdateList(t *testing.T) {
	tc := NewTestClient(t)
	tc.GET("/updates/ci", 403)
	tc.GET("/updates/ci/tag", 403)
	tc.GET("/updates/prod", 403)
	tc.GET("/updates/prod/tag", 403)
	tc.u.AllowedScopes = users.ScopeUpdatesR

	tc.GET("/updates/non-prod", 404)
	tc.GET("/updates/non-prod/tag", 404)

	s := func(data []byte) string {
		return strings.TrimSpace(string(data))
	}

	require.Nil(t, tc.fs.Updates.Ci.Rollouts.WriteFile("tag1", "update1", "rollout1", "foo"))
	require.Nil(t, tc.fs.Updates.Ci.Rollouts.WriteFile("tag1", "update2", "rollout1", "foo"))
	require.Nil(t, tc.fs.Updates.Ci.Rollouts.WriteFile("tag2", "update1", "rollout1", "foo"))
	require.Nil(t, tc.fs.Updates.Ci.Rollouts.WriteFile("tag2", "update3", "rollout1", "foo"))
	require.Nil(t, tc.fs.Updates.Prod.Rollouts.WriteFile("tag1", "update2", "rollout1", "foo"))
	require.Nil(t, tc.fs.Updates.Prod.Rollouts.WriteFile("tag4", "update42", "rollout1", "foo"))

	data := tc.GET("/updates/ci", 200)
	assert.Equal(t, `{"tag1":["update1","update2"],"tag2":["update1","update3"]}`, s(data))
	data = tc.GET("/updates/ci/tag1", 200)
	assert.Equal(t, `{"tag1":["update1","update2"]}`, s(data))
	data = tc.GET("/updates/ci/tag2", 200)
	assert.Equal(t, `{"tag2":["update1","update3"]}`, s(data))
	data = tc.GET("/updates/ci/tag4", 200) // tag not exists
	assert.Equal(t, "{}", s(data))
	data = tc.GET("/updates/prod", 200)
	assert.Equal(t, `{"tag1":["update2"],"tag4":["update42"]}`, s(data))
	data = tc.GET("/updates/prod/tag1", 200)
	assert.Equal(t, `{"tag1":["update2"]}`, s(data))
	data = tc.GET("/updates/prod/tag2", 200) // tag not exists
	assert.Equal(t, "{}", s(data))
	data = tc.GET("/updates/prod/tag4", 200)
	assert.Equal(t, `{"tag4":["update42"]}`, s(data))

	// Synthetic tag validation - create a bad tag on disk - request must still return 404
	require.Nil(t, tc.fs.Updates.Prod.Rollouts.WriteFile("bad^tag", "update42", "rollout1", "foo"))
	tc.GET("/updates/prod/bad^tag", 404)
}

func TestApiRolloutList(t *testing.T) {
	tc := NewTestClient(t)
	tc.GET("/updates/ci/tag/update/rollouts", 403)
	tc.GET("/updates/prod/tag/update/rollouts", 403)
	tc.u.AllowedScopes = users.ScopeUpdatesR

	tc.GET("/updates/non-prod/tag/update/rollouts", 404)

	s := func(data []byte) string {
		return strings.TrimSpace(string(data))
	}

	require.Nil(t, tc.fs.Updates.Ci.Rollouts.WriteFile("tag1", "update1", "rollout1", "foo"))
	require.Nil(t, tc.fs.Updates.Ci.Rollouts.WriteFile("tag1", "update1", "rollout2", "foo"))
	require.Nil(t, tc.fs.Updates.Ci.Rollouts.WriteFile("tag2", "update1", "rollout1", "foo"))
	require.Nil(t, tc.fs.Updates.Prod.Rollouts.WriteFile("tag1", "update2", "rollout4", "foo"))

	data := tc.GET("/updates/ci/tag1/update1/rollouts", 200)
	assert.Equal(t, `["rollout1","rollout2"]`, s(data))
	data = tc.GET("/updates/ci/tag2/update1/rollouts", 200)
	assert.Equal(t, `["rollout1"]`, s(data))
	data = tc.GET("/updates/ci/tag2/update2/rollouts", 200) // update not exists
	assert.Equal(t, "[]", s(data))
	data = tc.GET("/updates/ci/tag3/update1/rollouts", 200) // tag not exists
	assert.Equal(t, "[]", s(data))
	data = tc.GET("/updates/prod/tag1/update2/rollouts", 200)
	assert.Equal(t, `["rollout4"]`, s(data))
	data = tc.GET("/updates/ci/tag2/update2/rollouts", 200) // tag not exists
	assert.Equal(t, "[]", s(data))

	// Synthetic tag/update validation - create a bad tag/update on disk - request must still return 404
	require.Nil(t, tc.fs.Updates.Prod.Rollouts.WriteFile("bad^tag", "update42", "rollout1", "foo"))
	require.Nil(t, tc.fs.Updates.Prod.Rollouts.WriteFile("tag", "update=bad", "rollout1", "foo"))
	tc.GET("/updates/prod/bad^tag/update42/rollouts", 404)
	tc.GET("/updates/prod/tag/update=bad/rollouts", 404)
}

func TestApiRolloutGet(t *testing.T) {
	tc := NewTestClient(t)
	tc.GET("/updates/ci/tag/update/rollouts/rolling", 403)
	tc.GET("/updates/prod/tag/update/rollouts/stones", 403)
	tc.u.AllowedScopes = users.ScopeUpdatesR

	tc.GET("/updates/non-prod/tag/update/rollouts/rocks", 404)

	s := func(data []byte) string {
		return strings.TrimSpace(string(data))
	}

	require.Nil(t, tc.fs.Updates.Ci.Rollouts.WriteFile("tag1", "update1", "rollout1", `{"uuids":["123","xyz"]}`))
	require.Nil(t, tc.fs.Updates.Ci.Rollouts.WriteFile("tag1", "update2", "rollout2", `{"groups":["test","dev"]}`))
	require.Nil(t, tc.fs.Updates.Prod.Rollouts.WriteFile("tag", "update", "rollout", `{"uuids":["uh"],"groups":["oh"]}`))

	data := tc.GET("/updates/ci/tag1/update1/rollouts/rollout1", 200)
	assert.Equal(t, `{"uuids":["123","xyz"],"committed":false}`, s(data))
	data = tc.GET("/updates/ci/tag1/update2/rollouts/rollout2", 200)
	assert.Equal(t, `{"groups":["test","dev"],"committed":false}`, s(data))
	tc.GET("/updates/ci/tag1/update2/rollouts/rollout3", 404) // rollout not exists
	tc.GET("/updates/ci/tag1/update3/rollouts/rollout1", 404) // update not exists
	tc.GET("/updates/ci/tag2/update1/rollouts/rollout1", 404) // tag not exists
	data = tc.GET("/updates/prod/tag/update/rollouts/rollout", 200)
	assert.Equal(t, `{"uuids":["uh"],"groups":["oh"],"committed":false}`, s(data))

	// Synthetic tag/update/rollout validation - create a bad tag/update/rollout on disk - request must still return 404
	require.Nil(t, tc.fs.Updates.Prod.Rollouts.WriteFile("bad^tag", "update42", "rollout1", "foo"))
	require.Nil(t, tc.fs.Updates.Prod.Rollouts.WriteFile("tag", "update=bad", "rollout1", "foo"))
	require.Nil(t, tc.fs.Updates.Prod.Rollouts.WriteFile("tag", "update", "omg+", "foo"))
	tc.GET("/updates/prod/bad^tag/update42/rollouts/rollout1", 404)
	tc.GET("/updates/prod/tag/update=bad/rollouts/rollout1", 404)
	tc.GET("/updates/prod/tag/update/rollouts/omg+", 404)
}

func TestApiRolloutPut(t *testing.T) {
	tc := NewTestClient(t)
	tc.PUT("/updates/ci/tag/update/rollouts/rolling", 403, "{}")
	tc.PUT("/updates/prod/tag/update/rollouts/stones", 403, "{}")
	tc.u.AllowedScopes = users.ScopeUpdatesRU

	tc.PUT("/updates/non-prod/tag/update/rollouts/rocks", 404, "{}")

	tc.PUT("/updates/prod/tag/update/rollouts/rocks", 400, "{")
	tc.PUT("/updates/prod/tag/update/rollouts/rocks", 400, "{}")

	require.Nil(t, tc.fs.Updates.Ci.Ostree.WriteFile("tag1", "update1", "foo", "bar"))
	require.Nil(t, tc.fs.Updates.Prod.Ostree.WriteFile("tag2", "update2", "foo", "bar"))
	d, err := tc.gw.DeviceCreate("ci1", "pubkey1", false)
	require.Nil(t, err)
	require.Nil(t, d.CheckIn("", "tag1", "", ""))
	d, err = tc.gw.DeviceCreate("ci2", "pubkey1", false)
	require.Nil(t, err)
	require.Nil(t, d.CheckIn("", "tag1", "", ""))
	d, err = tc.gw.DeviceCreate("ci3", "pubkey1", false)
	require.Nil(t, err)
	require.Nil(t, d.CheckIn("", "tag2", "", ""))
	d, err = tc.gw.DeviceCreate("ci4", "pubkey1", false)
	require.Nil(t, err)
	require.Nil(t, d.CheckIn("", "tag2", "", ""))
	d, err = tc.gw.DeviceCreate("prod1", "pubkey2", true)
	require.Nil(t, err)
	require.Nil(t, d.CheckIn("", "tag2", "", ""))
	d, err = tc.gw.DeviceCreate("prod2", "pubkey2", true)
	require.Nil(t, err)
	require.Nil(t, d.CheckIn("", "tag2", "", ""))
	d, err = tc.gw.DeviceCreate("prod3", "pubkey2", true)
	require.Nil(t, err)
	require.Nil(t, d.CheckIn("", "tag2", "", ""))
	d, err = tc.gw.DeviceCreate("prod4", "pubkey2", true)
	require.Nil(t, err)
	require.Nil(t, d.CheckIn("", "tag3", "", ""))

	grp1 := "grp1"
	require.Nil(t, tc.api.PatchDeviceLabels(map[string]*string{"group": &grp1}, []string{"prod3", "prod4", "ci4"}))

	tc.PUT("/updates/ci/tag1/update1/rollouts/rocks", 202,
		`{"uuids":["ci1","ci2","ci3"]}`, "content-type", "application/json")
	tc.PUT("/updates/ci/tag1/update2/rollouts/rocks", 404,
		`{"uuids":["ci1","ci2"]}`, "content-type", "application/json")
	tc.PUT("/updates/ci/tag1/update1/rollouts/rocks", 409,
		`{"uuids":["ci1"]}`, "content-type", "application/json")
	tc.PUT("/updates/prod/tag2/update2/rollouts/rocks", 202,
		`{"uuids":["prod2"],"groups":["grp1"]}`, "content-type", "application/json")
	tc.PUT("/updates/prod/tag1/update1/rollouts/rocks", 404,
		`{"uuids":["prod2"],"groups":["grp1"]}`, "content-type", "application/json")

	s := func(data []byte) string {
		return strings.TrimSpace(string(data))
	}
	time.Sleep(50 * time.Millisecond) // Allow async database updates to finish

	data := tc.GET("/updates/ci/tag1/update1/rollouts/rocks", 200)
	assert.Equal(t, `{"uuids":["ci1","ci2","ci3"],"effective-uuids":["ci1","ci2"],"committed":true}`, s(data))
	data = tc.GET("/updates/prod/tag2/update2/rollouts/rocks", 200)
	assert.Equal(t, `{"uuids":["prod2"],"groups":["grp1"],"effective-uuids":["prod2","prod3"],"committed":true}`, s(data))
	dev, err := tc.api.DeviceGet("ci1")
	require.Nil(t, err)
	assert.Equal(t, "update1", dev.UpdateName)
	dev, err = tc.api.DeviceGet("ci2")
	require.Nil(t, err)
	assert.Equal(t, "update1", dev.UpdateName)
	dev, err = tc.api.DeviceGet("prod1")
	require.Nil(t, err)
	assert.Equal(t, "", dev.UpdateName)
	dev, err = tc.api.DeviceGet("prod2")
	assert.Nil(t, err)
	assert.Equal(t, "update2", dev.UpdateName)
	dev, err = tc.api.DeviceGet("prod3")
	require.Nil(t, err)
	assert.Equal(t, "update2", dev.UpdateName)

	// Synthetic tag/update/rollout validation - create a bad tag/update/rollout on disk - request must still return 404
	require.Nil(t, tc.fs.Updates.Prod.Rollouts.WriteFile("bad^tag", "update42", "rollout1", "foo"))
	require.Nil(t, tc.fs.Updates.Prod.Rollouts.WriteFile("tag", "update=bad", "rollout1", "foo"))
	require.Nil(t, tc.fs.Updates.Prod.Rollouts.WriteFile("tag", "update", "omg+", "foo"))
	tc.PUT("/updates/prod/bad^tag/update42/rollouts/gogogo", 404, "foo")
	tc.PUT("/updates/prod/tag/update=bad/rollouts/gogogo", 404, "foo")
	tc.PUT("/updates/prod/tag/update/rollouts/omg+", 404, "foo")
}

func TestApiRolloutDaemon(t *testing.T) {
	tc := NewTestClient(t)

	require.Nil(t, tc.fs.Auth.InitHmacSecret())
	db, err := apiStorage.NewDb(filepath.Join(t.TempDir(), apiStorage.DbFile))
	require.Nil(t, err)
	usersS, err := users.NewStorage(db, tc.fs)
	require.Nil(t, err)
	daemons := daemons.New(tc.ctx, tc.api, usersS, daemons.WithRolloverInterval(20*time.Millisecond))

	daemons.Start()
	defer daemons.Shutdown()
	tc.u.AllowedScopes = users.ScopeUpdatesR

	require.Nil(t, tc.fs.Updates.Ci.Ostree.WriteFile("tag1", "update1", "foo", "bar"))
	require.Nil(t, tc.fs.Updates.Prod.Ostree.WriteFile("tag2", "update2", "foo", "bar"))
	d, err := tc.gw.DeviceCreate("ci1", "pubkey1", false)
	require.Nil(t, err)
	require.Nil(t, d.CheckIn("", "tag1", "", ""))
	d, err = tc.gw.DeviceCreate("prod1", "pubkey2", true)
	require.Nil(t, err)
	require.Nil(t, d.CheckIn("", "tag2", "", ""))

	s := func(data []byte) string {
		return strings.TrimSpace(string(data))
	}

	// Emulate a non-committed rollout (file present, database not updated).
	require.Nil(t, tc.api.CreateRollout("tag1", "update1", "roll1", false, Rollout{Uuids: []string{"ci1"}}))
	require.Nil(t, tc.api.CreateRollout("tag2", "update2", "roll2", true, Rollout{Uuids: []string{"prod1"}}))

	// Before the watchdog daemon processing, rollouts are not yet committed.
	data := tc.GET("/updates/ci/tag1/update1/rollouts/roll1", 200)
	assert.Equal(t, `{"uuids":["ci1"],"committed":false}`, s(data))
	data = tc.GET("/updates/prod/tag2/update2/rollouts/roll2", 200)
	assert.Equal(t, `{"uuids":["prod1"],"committed":false}`, s(data))
	dev, err := tc.api.DeviceGet("ci1")
	require.Nil(t, err)
	assert.Equal(t, "", dev.UpdateName)
	dev, err = tc.api.DeviceGet("prod1")
	require.Nil(t, err)
	assert.Equal(t, "", dev.UpdateName)

	// After the watchdog daemon processing, rollouts are committed.
	time.Sleep(60 * time.Millisecond)
	data = tc.GET("/updates/ci/tag1/update1/rollouts/roll1", 200)
	assert.Equal(t, `{"uuids":["ci1"],"effective-uuids":["ci1"],"committed":true}`, s(data))
	data = tc.GET("/updates/prod/tag2/update2/rollouts/roll2", 200)
	assert.Equal(t, `{"uuids":["prod1"],"effective-uuids":["prod1"],"committed":true}`, s(data))
	dev, err = tc.api.DeviceGet("ci1")
	assert.Nil(t, err)
	assert.Equal(t, "update1", dev.UpdateName)
	dev, err = tc.api.DeviceGet("prod1")
	assert.Nil(t, err)
	assert.Equal(t, "update2", dev.UpdateName)
}

func TestApiUpdateTail(t *testing.T) {
	tc := NewTestClient(t)
	tc.GET("/updates/prod/tag1/update1/tail", 403)
	tc.u.AllowedScopes = users.ScopeUpdatesR

	d, err := tc.gw.DeviceCreate("test-device-1", "pubkey1", true)
	require.Nil(t, err)
	require.Nil(t, d.CheckIn("", "tag1", "", ""))
	d, err = tc.gw.DeviceCreate("test-device-2", "pubkey1", true)
	require.Nil(t, err)
	require.Nil(t, d.CheckIn("", "tag1", "", ""))
	d, err = tc.gw.DeviceCreate("test-device-3", "pubkey1", true)
	require.Nil(t, err)
	require.Nil(t, d.CheckIn("", "tag1", "", ""))
	_, err = tc.api.SetUpdateName("tag1", "update1", true, []string{"test-device-1", "test-device-2"}, nil)
	require.Nil(t, err)

	d1, err := tc.gw.DeviceGet("test-device-1")
	require.Nil(t, err)
	d2, err := tc.gw.DeviceGet("test-device-2")
	require.Nil(t, err)
	d3, err := tc.gw.DeviceGet("test-device-3")
	require.Nil(t, err)

	// Emulate a real HTTP client holding connection - something a test client apparently does not do.
	ctx, cancel := context.WithCancel(tc.ctx)
	tc.ctx = ctx

	// Before any events appear, check the correct error event is received.
	done := make(chan bool)
	rec := tc.DoAsync(httptest.NewRequest(http.MethodGet, "/v1/updates/prod/tag1/update1/tail", nil), done)
	time.Sleep(10 * time.Millisecond)
	expectedStream := `event: error
id: 0
retry: 1000
data: No rollout logs for this update yet.

`
	require.Equal(t, 200, rec.Code)
	require.Equal(t, expectedStream, rec.Body.String())
	tc.assertDone(done)

	events := generateUpdateEvents("uuid-1", "first", 1)
	require.Nil(t, d1.ProcessEvents(events))
	events = generateUpdateEvents("uuid-2", "second", 1)
	require.Nil(t, d2.ProcessEvents(events))
	events = generateUpdateEvents("uuid-3", "third", 1)
	require.Nil(t, d3.ProcessEvents(events))

	// Check that the original response did not change, meaning that it was closed by server.
	time.Sleep(10 * time.Millisecond)
	require.Equal(t, expectedStream, rec.Body.String())

	// rec1 is plain request, rec2 is request with resumption.
	done1 := make(chan bool)
	rec1 := tc.DoAsync(httptest.NewRequest(http.MethodGet, "/v1/updates/prod/tag1/update1/tail", nil), done1)
	done2 := make(chan bool)
	req2 := httptest.NewRequest(http.MethodGet, "/v1/updates/prod/tag1/update1/tail", nil)
	req2.Header.Add("Last-Event-ID", "1")
	rec2 := tc.DoAsync(req2, done2)
	time.Sleep(10 * time.Millisecond)
	// A previous error line should not appear in the new response.
	expectedStream1 := `event: log
id: 1
data: {"uuid":"test-device-1","correlationId":"uuid-1","target-name":"intel-corei7-64-lmp-23","status":"Download started","deviceTime":"2023-12-12T12:00:00"}

`
	expectedStream2 := `event: log
id: 2
data: {"uuid":"test-device-2","correlationId":"uuid-2","target-name":"intel-corei7-64-lmp-23","status":"Download started","deviceTime":"2023-12-12T12:00:00"}

`
	expectedStream1 += expectedStream2
	require.Equal(t, 200, rec1.Code)
	require.Equal(t, expectedStream1, rec1.Body.String())
	require.Equal(t, 200, rec2.Code)
	require.Equal(t, expectedStream2, rec2.Body.String())

	// Write to the file and check the new response bytes within the same connections.
	events = generateUpdateEvents("uuid-1", "forth", 1)
	require.Nil(t, d1.ProcessEvents(events))
	time.Sleep(10 * time.Millisecond)
	expectedStreamX := `event: log
id: 3
data: {"uuid":"test-device-1","correlationId":"uuid-1","target-name":"intel-corei7-64-lmp-23","status":"Download started","deviceTime":"2023-12-12T12:00:00"}

`
	expectedStream1 += expectedStreamX
	expectedStream2 += expectedStreamX
	require.Equal(t, expectedStream1, rec1.Body.String())
	require.Equal(t, expectedStream2, rec2.Body.String())
	tc.assertNotDone(done1)
	tc.assertNotDone(done2)

	// keepalive test
	saved := keepaliveResponseInterval
	keepaliveResponseInterval = 50 * time.Millisecond
	defer func() { keepaliveResponseInterval = saved }()
	done3 := make(chan bool)
	rec3 := tc.DoAsync(httptest.NewRequest(http.MethodGet, "/v1/updates/prod/tag1/update1/tail", nil), done3)
	time.Sleep(130 * time.Millisecond)
	expectedStream3 := expectedStream1 + keepaliveResponseText + keepaliveResponseText
	require.Equal(t, 200, rec3.Code)
	require.Equal(t, expectedStream3, rec3.Body.String())
	require.Nil(t, d1.ProcessEvents(events))
	time.Sleep(80 * time.Millisecond)
	expectedStreamY := strings.Replace(expectedStreamX, "id: 3", "id: 4", 1)
	expectedStream3 += expectedStreamY + keepaliveResponseText
	require.Equal(t, expectedStream3, rec3.Body.String())
	tc.assertNotDone(done3)

	cancel() // This is where we disconnect, closing all holding handlers.
	time.Sleep(10 * time.Millisecond)
	tc.assertDone(done1)
	tc.assertDone(done2)
	tc.assertDone(done3)

	// TODO: Add rollout tail tests
}

func TestApiDeviceDelete(t *testing.T) {
	tc := NewTestClient(t)

	// Create a device
	_, err := tc.gw.DeviceCreate("del-device", "pubkey", false)
	require.Nil(t, err)

	// No permission
	tc.DELETE("/devices/del-device", 403)

	// Wrong scope
	tc.u.AllowedScopes = users.ScopeDevicesR
	tc.DELETE("/devices/del-device", 403)

	// With delete scope
	tc.u.AllowedScopes = users.ScopeDevicesD

	// 404 for non-existent device
	tc.DELETE("/devices/no-such-device", 404)

	// Successful delete
	tc.DELETE("/devices/del-device", 204)

	// Verify device is gone
	tc.u.AllowedScopes = users.ScopeDevicesR
	tc.GET("/devices/del-device", 404)
}

func TestApiConfigsFactory(t *testing.T) {
	tc := NewTestClient(t)
	const (
		validConfig1  = `{"test":{"Value":"test config1"}}`
		validConfig2  = `{"test2":{"Value":"test config2"}}`
		invalidConfig = `{"test":{"Value1":"test config"}}`
	)

	t.Run("Default user scopes", func(t *testing.T) {
		tc.GET("/configs/factory", 403)
		tc.GET("/configs/factory/history", 403)
		tc.PUT("/configs/factory", 403, validConfig1)
	})

	tc.u.AllowedScopes = users.ScopeDevicesR
	t.Run("Read-only user scopes", func(t *testing.T) {
		assert.Equal(t, []byte(nil), tc.GET("/configs/factory", 204))
		assert.Equal(t, "[]\n", string(tc.GET("/configs/factory/history", 200)))
		tc.PUT("/configs/factory", 403, validConfig1)
		tc.u.AllowedScopes = users.ScopeDevicesRU
		tc.PUT("/configs/factory", 403, validConfig1)
		tc.u.AllowedScopes = users.ScopeDevicesRU | users.ScopeUpdatesR
		tc.PUT("/configs/factory", 403, validConfig1)
		tc.u.AllowedScopes = users.ScopeDevicesR | users.ScopeUpdatesRU
		tc.PUT("/configs/factory", 403, validConfig1)
	})

	tc.u.AllowedScopes = users.ScopeDevicesRU | users.ScopeUpdatesRU
	t.Run("Upload valid config", func(t *testing.T) {
		tc.PUT("/configs/factory", 204, validConfig1)
		assert.Equal(t, validConfig1+"\n", string(tc.GET("/configs/factory", 200)))
		assert.Equal(t, fmt.Sprintf("[%s]\n", validConfig1), string(tc.GET("/configs/factory/history", 200)))
	})

	t.Run("Update valid config", func(t *testing.T) {
		tc.PUT("/configs/factory", 204, validConfig2)
		assert.Equal(t, validConfig2+"\n", string(tc.GET("/configs/factory", 200)))
		assert.Equal(t, fmt.Sprintf("[%s,%s]\n", validConfig2, validConfig1), string(tc.GET("/configs/factory/history", 200)))
	})

	t.Run("Update invalid config", func(t *testing.T) {
		tc.PUT("/configs/factory", 400, invalidConfig)
		assert.Equal(t, validConfig2+"\n", string(tc.GET("/configs/factory", 200)))
		assert.Equal(t, fmt.Sprintf("[%s,%s]\n", validConfig2, validConfig1), string(tc.GET("/configs/factory/history", 200)))
	})

	t.Run("Update same config", func(t *testing.T) {
		tc.PUT("/configs/factory", 204, validConfig2)
		assert.Equal(t, validConfig2+"\n", string(tc.GET("/configs/factory", 200)))
		assert.Equal(t, fmt.Sprintf("[%s,%s]\n", validConfig2, validConfig1), string(tc.GET("/configs/factory/history", 200)))
	})
}

func TestApiConfigsUpload(t *testing.T) {
	tc := NewTestClient(t)

	// Extensive testing of the upload logic is a part of the storage/api tests.
	// Here we only need to test the Web part: handlers/transport/auth.
	validTarFiles := map[string]string{
		"factory/.journal":     "deadbeef:123456\nelvisalive:137137\n",
		"factory/deadbeef":     `{"test":{"Value":"test factory config"}}`,
		"group/beta/.journal":  "killbill:2003\n",
		"group/beta/killbill":  `{"samurai":{"Value":"test group config"}}`,
		"factory/elvisalive":   `{"test":{"Value":"test factory config latest version"}}`,
		"device/uuid/.journal": "",
	}

	t.Run("Failure on default user scopes", func(t *testing.T) {
		r := tarBuffer(t, validTarFiles)
		tc.PUT("/configs", 403, r, "Content-Type", "application/x-tar")
	})

	tc.u.AllowedScopes = users.ScopeDevicesR | users.ScopeUpdatesRU
	t.Run("Failure on updates read-write and devices read-only", func(t *testing.T) {
		r := tarBuffer(t, validTarFiles)
		tc.PUT("/configs", 403, r, "Content-Type", "application/x-tar")
	})

	tc.u.AllowedScopes = users.ScopeDevicesRU | users.ScopeUpdatesR
	t.Run("Failure on devices read-write and updates read-only", func(t *testing.T) {
		r := tarBuffer(t, validTarFiles)
		tc.PUT("/configs", 403, r, "Content-Type", "application/x-tar")
	})

	tc.u.AllowedScopes = users.ScopeDevicesRU | users.ScopeUpdatesRU
	t.Run("Success on devices and updates read-write and tar transport", func(t *testing.T) {
		r := tarBuffer(t, validTarFiles)
		tc.PUT("/configs", 200, r, "Content-Type", "application/x-tar")
	})

	t.Run("Success on devices and updates read-write and tar.gz transport", func(t *testing.T) {
		r := gzipBuffer(t, tarBuffer(t, validTarFiles))
		tc.PUT("/configs", 200, r, "Content-Type", "application/x-tar", "Content-Encoding", "gzip")
	})

	t.Run("Success on devices and updates read-write and gzip transport", func(t *testing.T) {
		r := gzipBuffer(t, tarBuffer(t, validTarFiles))
		tc.PUT("/configs", 200, r, "Content-Type", "application/gzip")
	})
}

func TestApiUpdateCreate(t *testing.T) {
	tc := NewTestClient(t)

	validTargets := `{"signed": {"targets": {"foo": {"custom": {"tags": ["main"]}}}}}`

	validTar := tarBuffer(t, map[string]string{
		"tuf/root.json":              `{"signed":{}}`,
		"tuf/targets.json":           validTargets,
		"ostree_repo/config":         "[core]\nrepo_version=1\n",
		"ostree_repo/refs/heads/foo": "abc123",
	})

	// Should require auth scope
	tc.POST("/updates/ci/main/v1.0", 403, bytes.NewReader(validTar.Bytes()),
		"Content-Type", "application/x-tar")

	tc.u.AllowedScopes = users.ScopeUpdatesRU

	// Valid tar with tuf + ostree_repo
	tc.POST("/updates/ci/main/v1.0", 201, bytes.NewReader(validTar.Bytes()),
		"Content-Type", "application/x-tar")

	// Verify files were extracted to the right place
	updatesDir := tc.fs.Config.UpdatesCiDir()
	root, err := os.ReadFile(filepath.Join(updatesDir, "main", "v1.0", "tuf", "root.json"))
	require.NoError(t, err)
	assert.Equal(t, `{"signed":{}}`, string(root))
	config, err := os.ReadFile(filepath.Join(updatesDir, "main", "v1.0", "ostree_repo", "config"))
	require.NoError(t, err)
	assert.Equal(t, "[core]\nrepo_version=1\n", string(config))

	// Valid tar with tuf + apps (no ostree_repo)
	appsTar := tarBuffer(t, map[string]string{
		"tuf/root.json":    `{"signed":{}}`,
		"tuf/targets.json": validTargets,
		"apps/myapp.json":  `{"name":"myapp"}`,
	})
	tc.POST("/updates/prod/main/v2.0", 201, bytes.NewReader(appsTar.Bytes()),
		"Content-Type", "application/x-tar")
	prodDir := tc.fs.Config.UpdatesProdDir()
	appData, err := os.ReadFile(filepath.Join(prodDir, "main", "v2.0", "apps", "myapp.json"))
	require.NoError(t, err)
	assert.Equal(t, `{"name":"myapp"}`, string(appData))

	// Valid tar with tuf + ostree_repo + apps (both present)
	bothTar := tarBuffer(t, map[string]string{
		"tuf/root.json":      `{"signed":{}}`,
		"tuf/targets.json":   validTargets,
		"ostree_repo/config": "[core]\n",
		"apps/myapp.json":    `{}`,
	})
	tc.POST("/updates/ci/main/v3.0", 201, bytes.NewReader(bothTar.Bytes()),
		"Content-Type", "application/x-tar")

	// Valid tar with tuf + ostree_repo + apps (both present)
	// BUT - targets.json has invalid tag. An update for "main" isn't going to work if the update
	// doesn't have a target with tagged with "main"
	bothTar = tarBuffer(t, map[string]string{
		"tuf/root.json":      `{"signed":{}}`,
		"tuf/targets.json":   `{"signed": {"targets": {"foo": {"custom": {"tags": ["invalid"]}}}}}`,
		"ostree_repo/config": "[core]\n",
		"apps/myapp.json":    `{}`,
	})
	tc.POST("/updates/ci/main/v3.1", 400, bytes.NewReader(bothTar.Bytes()),
		"Content-Type", "application/x-tar")

	// Missing tuf directory
	noTufTar := tarBuffer(t, map[string]string{
		"ostree_repo/config": "[core]\n",
	})
	data := tc.POST("/updates/ci/main/v-bad1", 400, bytes.NewReader(noTufTar.Bytes()),
		"Content-Type", "application/x-tar")
	assert.Contains(t, string(data), "invalid update archive")

	// Missing ostree_repo and apps
	noContentTar := tarBuffer(t, map[string]string{
		"tuf/root.json": `{"signed":{}}`,
	})
	data = tc.POST("/updates/ci/main/v-bad2", 400, bytes.NewReader(noContentTar.Bytes()),
		"Content-Type", "application/x-tar")
	assert.Contains(t, string(data), "invalid update archive")

	// Gzip-compressed tar via Content-Type
	gzTar := gzipBuffer(t, tarBuffer(t, map[string]string{
		"tuf/root.json":      `{"signed":{}}`,
		"tuf/targets.json":   validTargets,
		"ostree_repo/config": "[core]\n",
	}))
	tc.POST("/updates/ci/main/v4.0", 201, bytes.NewReader(gzTar.Bytes()),
		"Content-Type", "application/gzip")
	_, err = os.ReadFile(filepath.Join(updatesDir, "main", "v4.0", "tuf", "root.json"))
	require.NoError(t, err)

	// Gzip-compressed tar via Content-Encoding header
	gzTar2 := gzipBuffer(t, tarBuffer(t, map[string]string{
		"tuf/root.json":      `{"signed":{}}`,
		"tuf/targets.json":   validTargets,
		"ostree_repo/config": "[core]\n",
	}))
	tc.POST("/updates/ci/main/v5.0", 201, bytes.NewReader(gzTar2.Bytes()),
		"Content-Type", "application/x-tar",
		"Content-Encoding", "gzip")
	_, err = os.ReadFile(filepath.Join(updatesDir, "main", "v5.0", "tuf", "root.json"))
	require.NoError(t, err)

	// Invalid gzip stream
	tc.POST("/updates/ci/main/v-bad3", 500, strings.NewReader("not-gzip-data"),
		"Content-Type", "application/gzip")

	// Invalid update path params
	tc.POST("/updates/ci/../../etc/v1.0", 404, bytes.NewReader(validTar.Bytes()),
		"Content-Type", "application/x-tar")
}

var tarBuffer = storageTesting.CreateTarBuffer

func gzipBuffer(t *testing.T, data *bytes.Buffer) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	defer func() { require.NoError(t, gw.Close()) }()
	_, err := io.Copy(gw, data)
	require.NoError(t, err)
	return &buf
}
