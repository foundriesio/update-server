// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package gateway

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/foundriesio/dg-satellite/clock"
	"github.com/foundriesio/dg-satellite/context"
	"github.com/foundriesio/dg-satellite/server"
	baseStorage "github.com/foundriesio/dg-satellite/storage"
	storage "github.com/foundriesio/dg-satellite/storage/gateway"
)

type testClient struct {
	t   *testing.T
	db  *storage.DbHandle
	fs  *storage.FsHandle
	gw  *storage.Storage
	e   *echo.Echo
	log *slog.Logger

	uuid string
	cert *x509.Certificate
}

func (c testClient) Do(req *http.Request) *httptest.ResponseRecorder {
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{c.cert},
	}
	req = req.WithContext(context.CtxWithLog(req.Context(), c.log))
	rec := httptest.NewRecorder()
	c.e.ServeHTTP(rec, req)
	return rec
}

func (c testClient) GET(resource string, status int, headers ...string) []byte {
	req := httptest.NewRequest(http.MethodGet, resource, nil)
	c.marshalHeaders(headers, req)
	rec := c.Do(req)
	require.Equal(c.t, status, rec.Code)
	return rec.Body.Bytes()
}

func (c testClient) POST(resource string, status int, data any, headers ...string) []byte {
	req := httptest.NewRequest(http.MethodPost, resource, c.marshalBody(data))
	c.marshalHeaders(headers, req)
	rec := c.Do(req)
	require.Equal(c.t, status, rec.Code)
	return rec.Body.Bytes()
}

func (c testClient) PUT(resource string, status int, data any, headers ...string) []byte {
	req := httptest.NewRequest(http.MethodPut, resource, c.marshalBody(data))
	c.marshalHeaders(headers, req)
	rec := c.Do(req)
	require.Equal(c.t, status, rec.Code)
	return rec.Body.Bytes()
}

func (c testClient) marshalHeaders(headers []string, req *http.Request) {
	require.Zero(c.t, len(headers)%2, "Headers must be a sequence of names and values - even number")
	for i := 0; i < len(headers)/2; i++ {
		req.Header.Add(headers[i*2], headers[i*2+1])
	}
}

func (c testClient) marshalBody(data any) io.Reader {
	if s, ok := data.(string); ok {
		return strings.NewReader(s)
	} else if b, ok := data.([]byte); ok {
		return bytes.NewReader(b)
	} else {
		b, err := json.Marshal(data)
		require.Nil(c.t, err)
		return bytes.NewReader(b)
	}
}

func newTestClient(t *testing.T, isProd bool) *testClient {
	tmpDir := t.TempDir()
	fsS, err := storage.NewFs(tmpDir)
	require.Nil(t, err)
	db, err := storage.NewDb(fsS.Config.DbFile())
	require.Nil(t, err)
	gwS, err := storage.NewStorage(db, fsS)
	require.Nil(t, err)

	log, err := context.InitLogger("debug")
	require.Nil(t, err)

	e := server.NewEchoServer()
	RegisterHandlers(e, gwS, "https://does-not-matter")

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.Nil(t, err)

	uuid := rand.Text() // Base32 encoded 128-bit (16-byte, 26 chars) random string
	subj := pkix.Name{CommonName: uuid}
	if isProd {
		bc := pkix.AttributeTypeAndValue{
			Type:  businessCategoryOid,
			Value: businessCategoryProduction,
		}
		subj.Names = append(subj.Names, bc)           // Parsed names
		subj.ExtraNames = append(subj.ExtraNames, bc) // Marshalled names
	}
	cert := x509.Certificate{
		Subject:   subj,
		PublicKey: priv.Public(),
	}
	tc := testClient{
		t:   t,
		gw:  gwS,
		fs:  fsS,
		db:  db,
		e:   e,
		log: log,

		uuid: uuid,
		cert: &cert,
	}
	return &tc
}

func NewTestClient(t *testing.T) *testClient {
	return newTestClient(t, false)
}

func NewProdTestClient(t *testing.T) *testClient {
	return newTestClient(t, true)
}

func TestApiDevice(t *testing.T) {
	lastSeen := time.Now().Add(-1 * time.Second).Unix()
	tc := NewTestClient(t)
	deviceBytes := tc.GET("/device", 200)
	var device storage.Device
	require.Nil(t, json.Unmarshal(deviceBytes, &device))
	assert.Equal(t, tc.cert.Subject.CommonName, device.Uuid)
	assert.Less(t, lastSeen, device.LastSeen)
}

func TestApiProxy(t *testing.T) {
	tc := NewTestClient(t)
	resBytes := tc.POST("/app-proxy-url", 201, nil)
	proxyUrl, err := url.Parse(string(resBytes))
	require.Nil(t, err)
	token := proxyUrl.Query().Get("token")

	req := httptest.NewRequest(http.MethodHead, "/registry/v2/factory/repo/blobs/sha256:123", nil)
	rec := tc.Do(req)
	require.Equal(t, 401, rec.Code, rec.Body.String())

	req = httptest.NewRequest(http.MethodHead, "/registry/v2/factory/repo/blobs/sha256:123", nil)
	q := req.URL.Query()
	q.Add("token", token)
	req.URL.RawQuery = q.Encode()
	rec = tc.Do(req)
	require.Equal(t, 200, rec.Code, rec.Body.String())
}

func TestCheckIn(t *testing.T) {
	apps := "a,b,c"
	hash := "abcd"
	tag := "tag"
	target := "target"
	tc := NewTestClient(t)
	deviceBytes := tc.GET(
		"/device", 200, "x-ats-dockerapps", apps, "x-ats-ostreehash", hash, "x-ats-tags", tag, "x-ats-target", target)

	var d *storage.Device
	require.Nil(t, json.Unmarshal(deviceBytes, &d))
	assert.Equal(t, apps, d.Apps)
	assert.Equal(t, hash, d.OstreeHash)
	assert.Equal(t, tag, d.Tag)
	assert.Equal(t, target, d.TargetName)

	d, err := tc.gw.DeviceGet(tc.uuid)
	require.Nil(t, err)
	assert.Equal(t, apps, d.Apps)
	assert.Equal(t, hash, d.OstreeHash)
	assert.Equal(t, tag, d.Tag)
	assert.Equal(t, target, d.TargetName)

	// Check that fields are not erased on a partial update
	tag = "switch"
	apps = "a,b,d"
	_ = tc.GET("/device", 200, "x-ats-dockerapps", apps, "x-ats-tags", tag)

	d, err = tc.gw.DeviceGet(tc.uuid)
	require.Nil(t, err)
	assert.Equal(t, apps, d.Apps)
	assert.Equal(t, hash, d.OstreeHash)
	assert.Equal(t, tag, d.Tag)
	assert.Equal(t, target, d.TargetName)
}

func TestConfig(t *testing.T) {
	tc := NewTestClient(t)

	// advance the wall clock forward manually
	now := time.Now().Truncate(time.Second).UTC()
	clock.Now = func() time.Time {
		return now
	}
	defer func() { clock.Now = time.Now }()

	var lastModifiedAt time.Time
	getConfigSince := func(status int, ifModifiedSince time.Time) (cfg map[string]ConfigFile) {
		req := httptest.NewRequest(http.MethodGet, "/config", nil)
		if !ifModifiedSince.IsZero() {
			req.Header.Add("If-Modified-Since", ifModifiedSince.Format(time.RFC1123))
		}
		rec := tc.Do(req)
		require.Equal(t, status, rec.Code)
		if status == 200 {
			var err error
			require.Nil(t, json.Unmarshal(rec.Body.Bytes(), &cfg))
			lastModifiedAt, err = time.Parse(time.RFC1123, rec.Header().Get("Date"))
			lastModifiedAt = lastModifiedAt.UTC()
			require.Nil(t, err)
		}
		return
	}
	getConfig := func(status int) map[string]ConfigFile {
		return getConfigSince(status, time.Time{}) // Zero time
	}

	checkpoint := now
	tick := func(saveCheckpoint bool) {
		if saveCheckpoint {
			checkpoint = now
		}
		now = now.Add(time.Second)
	}
	tick(true)

	// No config
	cfg := getConfig(204)
	require.Equal(t, 0, len(cfg))
	require.True(t, lastModifiedAt.IsZero())
	cfg = getConfigSince(204, checkpoint)
	require.Equal(t, 0, len(cfg))
	tick(false) // This test changes no configs

	checkConfig := func(name, content string, onChanged ...string) {
		v, ok := cfg[name]
		require.True(t, ok)
		require.Equal(t, content, v.Value)
		require.Equal(t, len(onChanged), len(v.OnChanged))
		for idx, item := range onChanged {
			require.Equal(t, item, v.OnChanged[idx])
		}
	}
	checkTimestamp := func(isModified bool) {
		if isModified {
			require.Equal(t, now, lastModifiedAt)
			cfg1 := getConfigSince(200, checkpoint)
			require.Equal(t, cfg, cfg1)
			cfg1 = getConfigSince(304, now)
			require.Equal(t, 0, len(cfg1))
		} else {
			require.Equal(t, checkpoint, lastModifiedAt)
			cfg1 := getConfigSince(304, checkpoint)
			require.Equal(t, 0, len(cfg1))
		}
	}

	// Added factory configs
	require.Nil(t, tc.fs.Configs.WriteFactoryConfig(
		`{"foo":{"Value":"foo content"},"bar":{"Value":"bar content","OnChanged":["/bin/bar"]}}`, "", ""))
	cfg = getConfig(200)
	require.Equal(t, 2, len(cfg))
	checkConfig("foo", "foo content")
	checkConfig("bar", "bar content", "/bin/bar")
	checkTimestamp(true)
	tick(true)

	// Added device configs - override one factory config, adds one more
	require.Nil(t, tc.fs.Configs.WriteDeviceConfig(tc.uuid,
		`{"bar":{"Value":"bar device"},"baz":{"Value":"baz device"}}`, "", ""))
	cfg = getConfig(200)
	require.Equal(t, 3, len(cfg))
	checkConfig("foo", "foo content")
	checkConfig("bar", "bar device")
	checkConfig("baz", "baz device")
	checkTimestamp(true)
	tick(true)

	// Added group configs, group not set
	require.Nil(t, tc.fs.Configs.WriteGroupConfig("first",
		`{"baz":{"Value":"first baz"},"toe":{"Value":"first toe"}}`, "", ""))
	require.Nil(t, tc.fs.Configs.WriteGroupConfig("second",
		`{"bar":{"Value":"second bar"},"baz":{"Value":"second baz"}}`, "", ""))
	cfg = getConfig(200)
	require.Equal(t, 3, len(cfg))
	checkConfig("foo", "foo content")
	checkConfig("bar", "bar device")
	checkConfig("baz", "baz device")
	// Next test sets device group which were created above; hence the checkpoint advances despite no config change here.
	checkTimestamp(false)
	tick(true)

	// SQLite sets group_name_modified_at inside a trigger. Override it to our desired value.
	// A rudimentary test is to verify that the group_name_modified_at inside a table was set to some non-zero value.
	setGroupStmt, err := tc.db.Prepare("TestUpdateGroup",
		`UPDATE devices SET labels=jsonb_set(labels,'$.group',?) WHERE uuid=?`)
	require.Nil(t, err)
	setGroupModifiedStmt, err := tc.db.Prepare("TestUpdateGroupModified",
		"UPDATE devices SET group_name_modified_at=? WHERE uuid=? AND group_name_modified_at != 0")
	require.Nil(t, err)
	setGroup := func(group string) {
		_, err := setGroupStmt.Exec(group, tc.uuid)
		require.Nil(t, err)
		_, err = setGroupModifiedStmt.Exec(now.Unix(), tc.uuid)
		require.Nil(t, err)
	}

	// Set first group - adds two configs, one is overridden by device config
	setGroup("first")
	cfg = getConfig(200)
	require.Equal(t, 4, len(cfg))
	checkConfig("foo", "foo content")
	checkConfig("bar", "bar device")
	checkConfig("baz", "baz device")
	checkConfig("toe", "first toe")
	checkTimestamp(true)
	tick(true)

	// Set second group - adds one config, overrides one factory config, both are overridden by device config
	setGroup("second")
	cfg = getConfig(200)
	require.Equal(t, 3, len(cfg))
	checkConfig("foo", "foo content")
	checkConfig("bar", "bar device")
	checkConfig("baz", "baz device")
	checkTimestamp(true)
	tick(true)

	// Changed device config - remove one factory/group override, keep another group override, add one more config
	require.Nil(t, tc.fs.Configs.WriteDeviceConfig(tc.uuid,
		`{"ooh":{"Value":"ooh device"},"baz":{"Value":"baz device"}}`, "", ""))
	cfg = getConfig(200)
	require.Equal(t, 4, len(cfg))
	checkConfig("foo", "foo content")
	checkConfig("bar", "second bar")
	checkConfig("baz", "baz device")
	checkConfig("ooh", "ooh device")
	checkTimestamp(true)
	tick(true)

	// Changed group config - remove factory override, add one more config
	require.Nil(t, tc.fs.Configs.WriteGroupConfig("second",
		`{"tip":{"Value":"second tip","OnChanged":["/big/tip"]},"baz":{"Value":"second baz"}}`, "", ""))
	cfg = getConfig(200)
	require.Equal(t, 5, len(cfg))
	checkConfig("foo", "foo content")
	checkConfig("bar", "bar content", "/bin/bar")
	checkConfig("baz", "baz device")
	checkConfig("ooh", "ooh device")
	checkConfig("tip", "second tip", "/big/tip")
	checkTimestamp(true)
	tick(true)

	// Changed factory config - remove one config
	require.Nil(t, tc.fs.Configs.WriteFactoryConfig(
		`{"bar":{"Value":"bar content","OnChanged":["/bin/bar"]}}`, "", ""))
	cfg = getConfig(200)
	require.Equal(t, 4, len(cfg))
	checkConfig("bar", "bar content", "/bin/bar")
	checkConfig("baz", "baz device")
	checkConfig("ooh", "ooh device")
	checkConfig("tip", "second tip", "/big/tip")
	checkTimestamp(true)
	tick(true)

	// Set third group, no group config
	setGroup("third")
	cfg = getConfig(200)
	require.Equal(t, 3, len(cfg))
	checkConfig("bar", "bar content", "/bin/bar")
	checkConfig("baz", "baz device")
	checkConfig("ooh", "ooh device")
	checkTimestamp(true)
}

func TestConfigSota(t *testing.T) {
	tc := NewTestClient(t)
	_ = tc.GET("/device", 200) // auto-register before setting group

	setGroupStmt, err := tc.db.Prepare("TestUpdateGroup",
		`UPDATE devices SET labels=jsonb_set(labels,'$.group',?) WHERE uuid=?`)
	require.Nil(t, err)
	_, err = setGroupStmt.Exec("group", tc.uuid)
	require.Nil(t, err)

	marshalSota := func(cfg string) string {
		jsonCfg, e := json.Marshal(cfg)
		require.Nil(t, e)
		return fmt.Sprintf(`{"%s":{"Value":%s}}`, storage.ConfigSotaOverride, string(jsonCfg))
	}

	cfg := marshalSota("[pacman]\ntags='factory'\napps='factory'\n[madman]\nfoo='bar'\n")
	require.Nil(t, tc.fs.Configs.WriteFactoryConfig(cfg, "", ""))
	cfg = marshalSota("[pacman]\ntags='group'\n[madman]\nbar='baz'\n")
	require.Nil(t, tc.fs.Configs.WriteGroupConfig("group", cfg, "", ""))
	cfg = marshalSota("[pacman]\napps='device'\n[badman]\nfoo='bar'\n")
	require.Nil(t, tc.fs.Configs.WriteDeviceConfig(tc.uuid, cfg, "", ""))

	// TOML library uses double-quotes for values, sorts everything alphabetically, and puts spaces around equality.
	mergedCfg := marshalSota(`[badman]
foo = "bar"

[madman]
bar = "baz"
foo = "bar"

[pacman]
apps = "device"
tags = "group"
`)
	beforeRequest := time.Now().Unix()
	serverCfg := strings.Trim(string(tc.GET("/config", 200)), "\n")
	require.Equal(t, mergedCfg, serverCfg)

	// Verify that the applied config envelope was persisted.
	raw, err := tc.fs.Devices.ReadFile(tc.uuid, storage.ConfigAppliedFile)
	require.Nil(t, err)
	var applied baseStorage.AppliedConfigs
	require.Nil(t, json.Unmarshal([]byte(raw), &applied))
	assert.GreaterOrEqual(t, applied.AppliedAt, beforeRequest)
	assert.LessOrEqual(t, applied.AppliedAt, time.Now().Unix())
	require.Contains(t, applied.Files, storage.ConfigSotaOverride)
	// The merged TOML value stored in the envelope should match what the server sent.
	assert.Equal(t, mergedCfg, fmt.Sprintf(`{"%s":{"Value":%s}}`,
		storage.ConfigSotaOverride,
		func() string {
			b, e := json.Marshal(applied.Files[storage.ConfigSotaOverride].Value)
			require.Nil(t, e)
			return string(b)
		}()),
	)
}

func TestInfo(t *testing.T) {
	akInfo := "[config]\nkey=value"
	hwInfo := `{"key":"value"}`
	hwInfoBad := `{key=value}`
	nwInfo := `{"hostname":"example.org"}`
	nwInfoBad := `{"hostname":123}`
	stInfo := `{"deviceTime":"2025-09-12T10:00:00Z"}`
	stInfo1 := `{"deviceTime":"2025-09-12T10:00:05Z"}`
	stInfoBad := `{"deviceTime":"2025-09-12 10:00:00"}`

	tc := NewTestClient(t)
	_ = tc.PUT("/system_info", 200, hwInfo)
	_ = tc.PUT("/system_info", 400, hwInfoBad)
	_ = tc.PUT("/system_info/config", 200, akInfo)
	_ = tc.PUT("/system_info/network", 200, nwInfo)
	_ = tc.PUT("/system_info/network", 400, nwInfoBad)
	_ = tc.POST("/apps-states", 200, stInfo)
	_ = tc.POST("/apps-states", 200, stInfo1)
	_ = tc.POST("/apps-states", 400, stInfoBad)

	aboveLimit := string(make([]byte, 100*1024+1)) // 100KB of zeroes + one byte
	_ = tc.PUT("/system_info/config", 413, aboveLimit)

	data, err := tc.fs.Devices.ReadFile(tc.uuid, storage.AktomlFile)
	assert.Nil(t, err)
	assert.Equal(t, akInfo, data)
	data, err = tc.fs.Devices.ReadFile(tc.uuid, storage.HwInfoFile)
	assert.Nil(t, err)
	assert.Equal(t, hwInfo, data)
	data, err = tc.fs.Devices.ReadFile(tc.uuid, storage.NetInfoFile)
	assert.Nil(t, err)
	assert.Equal(t, nwInfo, data)

	states, err := tc.fs.Devices.ListFiles(tc.uuid, storage.StatesPrefix, true)
	require.Nil(t, err)
	assert.Equal(t, 2, len(states))
	exp := []string{stInfo, stInfo1}
	for idx, name := range states {
		data, err = tc.fs.Devices.ReadFile(tc.uuid, name)
		assert.Nil(t, err)
		assert.Equal(t, exp[idx], data)
	}

	// apps states rollover
	for i := 0; i < 15; i++ {
		_ = tc.POST("/apps-states", 200, stInfo1)
	}
	states, err = tc.fs.Devices.ListFiles(tc.uuid, storage.StatesPrefix, true)
	require.Nil(t, err)
	assert.Equal(t, 10, len(states))
	for _, name := range states {
		data, err = tc.fs.Devices.ReadFile(tc.uuid, name)
		assert.Nil(t, err)
		assert.Equal(t, stInfo1, data)
	}
}

func TestEvents(t *testing.T) {
	var (
		eventSatus = `{"id":"dead","deviceTime":"2023-12-12T12:00:00Z",` +
			`"event":{"correlationId":"feed","ecu":"","targetName":"metam","version":"42"},` +
			`"eventType":{"id":"satus","version":123}}`
		eventFinis = `{"id":"beaf","deviceTime":"2023-12-12T12:00:42Z",` +
			`"event":{"correlationId":"feed","ecu":"","targetName":"metam","version":"42"},` +
			`"eventType":{"id":"finis","version":123}}`
		eventBadDate = `{"id":"dodo","deviceTime":"omghf",` +
			`"event":{"correlationId":"feed","ecu":"","targetName":"metam","version":"42"},` +
			`"eventType":{"id":"dies","version":123}}`
		eventFixedDate = strings.Replace(eventBadDate, "omghf", time.Now().UTC().Format(time.RFC3339), 1)
		eventBadId     = `{"id":"","deviceTime":"2023-12-12T12:00:55Z",` +
			`"event":{"correlationId":"feed","ecu":"","targetName":"metam","version":"42"},` +
			`"eventType":{"id":"fraus","version":123}}`
		eventBadCorrId = `{"id":"kiwi","deviceTime":"2023-12-12T12:00:55Z",` +
			`"event":{"correlationId":"","ecu":"","targetName":"metam","version":"42"},` +
			`"eventType":{"id":"fraus","version":123}}`

		eventsGood    = fmt.Sprintf(`[%s,%s]`, eventSatus, eventFinis)
		eventsBadData = fmt.Sprintf(`[%s,%s,%s]`, eventBadDate, eventBadId, eventBadCorrId)
		eventsBadJson = "here we go"
	)

	fmt.Println(eventsGood)
	tc := NewTestClient(t)
	_ = tc.POST("/events", 200, eventsGood)
	_ = tc.POST("/events", 200, eventsBadData)
	_ = tc.POST("/events", 400, eventsBadJson)

	eventsFiles, err := tc.fs.Devices.ListFiles(tc.uuid, storage.EventsPrefix, true)
	require.Nil(t, err)
	assert.Equal(t, 1, len(eventsFiles))
	eventsSaved, err := tc.fs.Devices.ReadFile(tc.uuid, eventsFiles[0])
	assert.Nil(t, err)
	assert.Equal(t, fmt.Sprintf("%s\n%s\n%s\n", eventSatus, eventFinis, eventFixedDate), eventsSaved)
}

func TestTufMeta(t *testing.T) {
	tcCi42 := NewTestClient(t)
	tcCi137 := NewTestClient(t)
	tcProd42 := NewProdTestClient(t)
	tcProd137 := NewProdTestClient(t)

	// Positive test bed
	tests := []struct {
		tc     *testClient // CI vs prod
		name   string      // test name and file data
		tag    string      // x-ats-tags header value
		update string      // update name, set for the device
		role   string      // URL's leaf path and file name
	}{
		{tcCi42, "CI test 1.root.json", "test", "42", "1.root.json"},
		{tcCi42, "CI test 3.root.json", "test", "42", "3.root.json"},
		{tcCi42, "CI test timestamp.json", "test", "42", "timestamp.json"},
		{tcCi42, "CI test snapshot.json", "test", "42", "snapshot.json"},
		{tcCi42, "CI test targets.json", "test", "42", "targets.json"},
		{tcCi137, "CI test 137 targets.json", "test", "137", "targets.json"},
		{tcCi137, "CI beta targets.json", "beta", "137", "targets.json"},
		{tcProd42, "Prod beta 1.root.json", "beta", "42", "1.root.json"},
		{tcProd137, "Prod prod 1.root.json", "prod", "137", "1.root.json"},
		{tcProd137, "Prod beta timestamp.json", "beta", "137", "timestamp.json"},
		{tcProd137, "Prod prod snapshot.json", "prod", "137", "snapshot.json"},
		{tcProd42, "Prod frog targets.json", "frog", "42", "targets.json"},
	}

	// Pre-create devices and set their update names before tests
	visited := make(map[*testClient]string, 4)
	for _, ts := range tests {
		if update, ok := visited[ts.tc]; ok {
			// Update must be equal for the same device across test cases.
			require.Equal(t, update, ts.update, ts.name)
		} else {
			visited[ts.tc] = ts.update
			_ = ts.tc.GET("/device", 200) // This creates the device via auto-register
			stmt, err := ts.tc.db.Prepare("TestUpdateUpdate", "UPDATE devices SET update_name=? WHERE uuid=?")
			require.Nil(t, err, ts.name)
			_, err = stmt.Exec(ts.update, ts.tc.cert.Subject.CommonName)
			require.Nil(t, err, ts.name)
		}
	}

	// Pre-create TUF data before tests
	var err error
	for _, ts := range tests {
		switch ts.tc {
		case tcCi42, tcCi137:
			err = ts.tc.fs.Updates.Ci.Tuf.WriteFile(ts.tag, ts.update, ts.role, ts.name)
		case tcProd42, tcProd137:
			err = ts.tc.fs.Updates.Prod.Tuf.WriteFile(ts.tag, ts.update, ts.role, ts.name)
		}
		require.Nil(t, err, ts.name)
	}

	// Finally, run the test
	for _, ts := range tests {
		t.Run(ts.name, func(t *testing.T) {
			ts.tc.t = t // Use sub-test testing handle
			roleBytes := ts.tc.GET("/repo/"+ts.role, 200, "x-ats-tags", ts.tag)
			assert.Equal(t, ts.name, string(roleBytes))
		})
		ts.tc.t = t // Restore parent test testing handle
	}

	// A few negative tests
	t.Run("Missing five.root.json", func(t *testing.T) {
		tcCi42.t = t
		_ = tcCi42.GET("/repo/five.root.json", 404, "x-ats-tags", "test")
	})
	t.Run("Missing 5.root.json", func(t *testing.T) {
		tcCi42.t = t
		_ = tcCi42.GET("/repo/5.root.json", 404, "x-ats-tags", "test")
	})
	t.Run("Missing targets.json for non-existing tag", func(t *testing.T) {
		tcCi42.t = t
		_ = tcCi42.GET("/repo/targets.json", 404, "x-ats-tags", "zero")
	})
	t.Run("Missing targets.json for existing tag but not matching update", func(t *testing.T) {
		tcCi42.t = t
		_ = tcCi42.GET("/repo/targets.json", 404, "x-ats-tags", "beta")
	})
}

func TestOstree(t *testing.T) {
	tcCi42 := NewTestClient(t)
	tcCi137 := NewTestClient(t)
	tcProd42 := NewProdTestClient(t)
	tcProd137 := NewProdTestClient(t)

	// Positive test bed
	tests := []struct {
		tc     *testClient // CI vs prod
		name   string      // test name and file data
		tag    string      // x-ats-tags header value
		update string      // update name, set for the device
		path   string      // URL's leaf path and file name
	}{
		{tcCi42, "CI test config", "test", "42", "config"},
		{tcCi42, "CI test objects", "test", "42", "objects/foo"},
		{tcCi42, "CI test deltas", "test", "42", "deltas/foo"},
		{tcCi42, "CI test delta indexes", "test", "42", "delta-indexes/foo"},
		{tcCi42, "CI test delta stats", "test", "42", "delta-stats/foo"},
		{tcCi42, "CI test summary", "test", "42", "summary"},
		{tcCi137, "CI beta config", "beta", "137", "config"},
		{tcCi137, "CI beta objects", "beta", "137", "objects/bar"},
		{tcProd42, "Prod beta config", "beta", "42", "config"},
		{tcProd42, "Prod beta objects", "beta", "42", "objects/bar"},
		{tcProd137, "Prod prod config", "prod", "137", "config"},
		{tcProd137, "Prod prod objects", "prod", "137", "objects/foo"},
	}

	// Pre-create devices and set their update names before tests
	visited := make(map[*testClient][2]string, 4)
	for _, ts := range tests {
		if tagAndUpdate, ok := visited[ts.tc]; ok {
			// Update and tag must be equal for the same device across test cases.
			require.Equal(t, tagAndUpdate[0], ts.tag, ts.name)
			require.Equal(t, tagAndUpdate[1], ts.update, ts.name)
		} else {
			visited[ts.tc] = [2]string{ts.tag, ts.update}
			_ = ts.tc.GET("/device", 200) // This creates the device via auto-register
			stmt, err := ts.tc.db.Prepare("TestUpdateUpdate", "UPDATE devices SET update_name=?, tag=? WHERE uuid=?")
			require.Nil(t, err, ts.name)
			_, err = stmt.Exec(ts.update, ts.tag, ts.tc.cert.Subject.CommonName)
			require.Nil(t, err, ts.name)
		}
	}

	writeFile := func(h baseStorage.UpdatesFsHandle, tag, update, path, content string) error {
		if parts := strings.Split(path, "/"); len(parts) > 1 {
			require.Equal(t, 2, len(parts), content) // Only level 1 depth in tests
			if err := os.MkdirAll(h.FilePath(tag, update, parts[0]), 0o750); err != nil {
				return err
			}
		}
		return h.WriteFile(tag, update, path, content)
	}

	// Pre-create TUF data before tests
	var err error
	for _, ts := range tests {
		switch ts.tc {
		case tcCi42, tcCi137:
			err = writeFile(ts.tc.fs.Updates.Ci.Ostree, ts.tag, ts.update, ts.path, ts.name)
		case tcProd42, tcProd137:
			err = writeFile(ts.tc.fs.Updates.Prod.Ostree, ts.tag, ts.update, ts.path, ts.name)
		}
		require.Nil(t, err, ts.name)
	}

	// Finally, run the test
	for _, ts := range tests {
		t.Run(ts.name, func(t *testing.T) {
			ts.tc.t = t // Use sub-test testing handle
			fileBytes := ts.tc.GET("/ostree/"+ts.path, 200)
			assert.Equal(t, ts.name, string(fileBytes))
		})
		ts.tc.t = t // Restore parent test testing handle
	}

	// A few negative tests
	t.Run("Wrong endpoint", func(t *testing.T) {
		tcCi42.t = t
		_ = tcCi42.GET("/ostree/foo", 404)
	})
	t.Run("Missing objects", func(t *testing.T) {
		tcCi42.t = t
		_ = tcCi42.GET("/ostree/objects/bar", 404)
	})
	t.Run("Download URLs", func(t *testing.T) {
		tcCi42.t = t
		body := tcCi42.POST("/ostree/download-urls", 204, nil)
		assert.Equal(t, "", string(body))
	})
	t.Run("Summary signature", func(t *testing.T) {
		tcCi42.t = t
		_ = tcCi42.GET("/ostree/summary.sig", 404)
	})
}

func TestApiFiotest(t *testing.T) {
	tc := NewTestClient(t)

	content := `{"name": "test-1"}`
	resp := tc.POST("/tests", 201, []byte(content))
	testid := string(resp)

	content = `{
			"status": "PASSED",
			"details": "detail x",
			"results": [
				{
					"name": "tr-1",
					"status": "FAILED"
				},
				{
					"name": "tr-2",
					"status": "PASSED",
					"local_ts": 1597802911.1365469,
					"details": "tr2-detail",
					"metrics": {
						"m1": 12,
						"m2": 42.1
					}
				}
			],
			"artifacts": ["console.txt"]
		}`
	out := tc.PUT("/tests/"+testid, 200, []byte(content))

	type signedUrl struct {
		Url         string `json:"url"`
		ContentType string `json:"content-type"`
	}
	var urls map[string]signedUrl
	require.Nil(t, json.Unmarshal(out, &urls))
	for name, signed := range urls {
		tc.PUT(signed.Url, 200, []byte(name+"BLAH"))
	}

	prefix := baseStorage.TestArtifactsPrefix + "-" + testid + "_"
	files, err := tc.fs.Devices.ListFiles(tc.cert.Subject.CommonName, prefix, true)
	require.Nil(t, err)
	require.Len(t, files, 1)
	require.Equal(t, prefix+"console.txt", files[0])
}
