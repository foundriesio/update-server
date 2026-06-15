// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package gateway

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	mrand "math/rand"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/api"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type UpdateEvents []storage.DeviceUpdateEvent

var genEventType = map[int]string{
	0: "EcuDownloadStarted",
	1: "EcuDownloadCompleted",
	2: "EcuInstallationStarted",
	3: "EcuInstallationApplied",
	4: "EcuInstallationCompleted",
}

func (ue UpdateEvents) generate(pack string, num int) UpdateEvents {
	if num > 5 {
		num = 5 // Protect against rogue tests. We only support at most 5 events per correlation ID below.
	}
	corId := rand.Text()
	events := make([]storage.DeviceUpdateEvent, num)
	for i := 0; i < num; i++ {
		var success *bool
		eventType := genEventType[i]
		if i == num-1 {
			var asuccess bool
			success = &asuccess
			// A last (failed) event must be EcuDownloadCompleted or EcuInstallationCompleted
			switch num {
			case 1:
				eventType = genEventType[1]
			case 3, 4:
				eventType = genEventType[4]
			case 5:
				asuccess = true
			}
		}
		events[i] = storage.DeviceUpdateEvent{
			Id:         fmt.Sprintf("%d_%s", i, corId),
			DeviceTime: "2023-12-12T12:00:00",
			Event: storage.DeviceEvent{
				CorrelationId: corId,
				Ecu:           "",
				// The last event in a pack is failed, unless there are 5 events (then all events are success).
				Success:    success,
				TargetName: "intel-corei7-64-lmp-23",
				Version:    "23",
				Details:    pack,
			},
			EventType: storage.DeviceEventType{
				Id:      eventType,
				Version: 0,
			},
		}
	}
	return events
}

func TestStorage(t *testing.T) {
	tmpdir := t.TempDir()
	dbFile := filepath.Join(tmpdir, "sql.db")
	db, err := storage.NewDb(dbFile)
	require.Nil(t, err)
	t.Cleanup(func() {
		require.Nil(t, db.Close())
	})
	fs, err := storage.NewFs(tmpdir)
	require.Nil(t, err)

	s, err := NewStorage(db, fs)
	require.Nil(t, err)

	d, err := s.DeviceGet("does not exist")
	require.Nil(t, err)
	require.Nil(t, d)

	uuid := "1234-567-890"
	d, err = s.DeviceCreate(uuid, "pubkey")
	require.Nil(t, err)

	d2, err := s.DeviceGet(uuid)
	require.Nil(t, err)
	require.Equal(t, d.PubKey, d2.PubKey)

	time.Sleep(time.Second)
	require.Nil(t, d2.CheckIn("target", "tag", "hash", ""))
	d2, err = s.DeviceGet(uuid)
	require.Nil(t, err)
	require.Less(t, d.LastSeen, d2.LastSeen)

	require.Nil(t, d2.PutFile(storage.AktomlFile, "test content"))
	content, err := fs.Devices.ReadFile(d2.Uuid, storage.AktomlFile)
	require.Nil(t, err)
	require.Equal(t, "test content", content)

	require.Nil(t, fs.Configs.WriteFactoryConfig("factory config", "alice", "test"))
	require.Nil(t, fs.Configs.WriteGroupConfig("grp", "group config", "bob", "save:it"))
	require.Nil(t, fs.Configs.WriteDeviceConfig(d2.Uuid, "device config", "bob", "this thing"))

	time.Sleep(10 * time.Millisecond)
	now := time.Now().Truncate(time.Second).Add(time.Second).Unix()

	d3 := Device{Uuid: "fake", storage: *s}
	cfgs, ts, err := d3.GetConfigs()
	require.Nil(t, err)
	require.Equal(t, "factory config", cfgs[0].RawFiles)
	require.Nil(t, cfgs[1])
	require.Nil(t, cfgs[2])
	require.Less(t, ts, now)
	require.Greater(t, ts, now-2)

	cfgs, ts, err = d2.GetConfigs()
	require.Nil(t, err)
	require.Equal(t, "factory config", cfgs[0].RawFiles)
	require.Nil(t, cfgs[1])
	require.Equal(t, "device config", cfgs[2].RawFiles)
	require.Less(t, ts, now)
	require.Greater(t, ts, now-2)

	d2.GroupName = "grp"
	cfgs, ts, err = d2.GetConfigs()
	require.Nil(t, err)
	require.Equal(t, "factory config", cfgs[0].RawFiles)
	require.Equal(t, "group config", cfgs[1].RawFiles)
	require.Equal(t, "device config", cfgs[2].RawFiles)
	require.Less(t, ts, now)
	require.Greater(t, ts, now-2)
}

func Test_ProcessEvents(t *testing.T) {
	tmpdir := t.TempDir()
	dbFile := filepath.Join(tmpdir, "sql.db")
	db, err := storage.NewDb(dbFile)
	require.Nil(t, err)
	t.Cleanup(func() {
		require.Nil(t, db.Close())
	})
	fs, err := storage.NewFs(tmpdir)
	require.Nil(t, err)

	s, err := NewStorage(db, fs)
	require.Nil(t, err)

	// Create fake device
	id := rand.Text()
	d, err := s.DeviceCreate(id, "pubkey")
	require.Nil(t, err)
	d.UpdateName = "update"
	d.Tag = "tag"

	stmt, err := db.Prepare("TestProcessEvents", "UPDATE devices SET update_name=?, tag=? WHERE uuid=?")
	require.Nil(t, err)
	_, err = stmt.Exec(d.UpdateName, d.Tag, d.Uuid)
	require.Nil(t, err)

	var events UpdateEvents
	expectedStatusLog := ""
	appendExpectedStatusLog := func(events UpdateEvents) {
		for _, ev := range events {
			st := ev.ParseStatus()
			st.Uuid = d.Uuid
			bytes, err := json.Marshal(st)
			require.Nil(t, err)
			expectedStatusLog += string(bytes) + "\n"
		}
	}
	for i := 0; i < s.maxEvents+3; i++ {
		pack := fmt.Sprintf("test-%d", i)
		events = events.generate(pack, i%4+2)
		appendExpectedStatusLog(events)
		require.Nil(t, d.ProcessEvents(events))
		time.Sleep(4 * time.Millisecond)
	}

	validate := func(files []string, skip int) {
		require.Equal(t, s.maxEvents, len(files))
		for i, name := range files {
			pack := fmt.Sprintf("test-%d", i+skip) // Some initial events must get stripped
			content, err := fs.Devices.ReadFile(d.Uuid, name)
			require.Nil(t, err)
			for _, line := range strings.Split(content, "\n") {
				if len(line) == 0 {
					continue
				}
				var evt storage.DeviceUpdateEvent
				require.Nil(t, json.Unmarshal([]byte(line), &evt))
				require.Equal(t, pack, evt.Event.Details)
			}
		}
		actualStatusLog, err := fs.Updates.Logs.ReadFile(d.Tag, d.UpdateName, storage.LogRolloutsFile)
		require.Nil(t, err)
		require.Equal(t, expectedStatusLog, actualStatusLog)
	}

	files, err := fs.Devices.ListFiles(d.Uuid, storage.EventsPrefix, true)
	require.Nil(t, err)
	validate(files, 3)

	// Special case - some events roll over to the next pack.
	lastEventCorrId := events[0].Event.CorrelationId
	lastEventPack := events[0].Event.Details
	newPack := fmt.Sprintf("test-%d", s.maxEvents+3)
	events = events.generate(newPack, 5)
	events[0].Event.CorrelationId = lastEventCorrId
	events[0].Event.Details = lastEventPack
	appendExpectedStatusLog(events) // These statuses are quite screwed; but that's fine for a test.
	require.Nil(t, d.ProcessEvents(events))

	files, err = fs.Devices.ListFiles(d.Uuid, storage.EventsPrefix, true)
	require.Nil(t, err)
	validate(files, 4)

	// TODO: Add fine-grained unit tests for SaveAppsStates
}

func Benchmark_ProcessEvents(b *testing.B) {
	tmpdir := b.TempDir()
	dbFile := filepath.Join(tmpdir, "sql.db")
	db, err := storage.NewDb(dbFile)
	require.Nil(b, err)
	b.Cleanup(func() {
		require.Nil(b, db.Close())
	})
	fs, err := storage.NewFs(tmpdir)
	require.Nil(b, err)

	s, err := NewStorage(db, fs)
	require.Nil(b, err)

	// Create fake devices
	var devices []*Device
	for i := 0; i < 10; i++ {
		id := rand.Text()
		d, err := s.DeviceCreate(id, "pubkey")
		require.Nil(b, err)
		devices = append(devices, d)
	}
	require.Nil(b, err)

	b.StartTimer()
	var events UpdateEvents
	for i := 0; i < 100000; i++ {
		events = events.generate("test", 5)
		deviceIdx := mrand.Intn(len(devices) - 1)
		require.Nil(b, devices[deviceIdx].ProcessEvents(events))
	}
	b.StopTimer()
}

// Benchmark_CheckIn simulates 100 random device checking in 100_000 times
func Benchmark_CheckIn(b *testing.B) {
	tmpdir := b.TempDir()
	dbFile := filepath.Join(tmpdir, "sql.db")
	db, err := storage.NewDb(dbFile)
	require.Nil(b, err)
	b.Cleanup(func() {
		require.Nil(b, db.Close())
	})
	fs, err := storage.NewFs(tmpdir)
	require.Nil(b, err)

	s, err := NewStorage(db, fs)
	require.Nil(b, err)

	// Create fake devices
	var devices []*Device
	for range 100 {
		id := rand.Text()
		d, err := s.DeviceCreate(id, "pubkey"+id)
		require.Nil(b, err)
		devices = append(devices, d)
	}

	b.StartTimer()
	for range 100000 {
		deviceIdx := mrand.Intn(len(devices) - 1)
		require.Nil(b, devices[deviceIdx].CheckIn("target", "tag", "hash", ""))
	}
	b.StopTimer()
}

func Test_Fiotest(t *testing.T) {
	tmpdir := t.TempDir()
	dbFile := filepath.Join(tmpdir, "sql.db")
	db, err := storage.NewDb(dbFile)
	require.Nil(t, err)
	t.Cleanup(func() {
		require.Nil(t, db.Close())
	})
	fs, err := storage.NewFs(tmpdir)
	require.Nil(t, err)

	s, err := NewStorage(db, fs)
	require.Nil(t, err)

	// Create fake device
	id := uuid.New().String()
	d, err := s.DeviceCreate(id, "pubkey")
	require.Nil(t, err)

	require.Nil(t, d.TestCreate("intel-corei7-64-lmp-23", "test1", "test1-id"))
	require.Nil(t, d.TestCreate("intel-corei7-64-lmp-23", "test1", "test2-id"))

	require.Nil(t, d.TestComplete("test1-id", "PASSED", "details", nil))

	results := []storage.TargetTestResult{
		{
			Name:    "res1",
			Status:  "PASSED",
			Details: "details",
		},
	}
	require.Nil(t, d.TestComplete("test2-id", "FAILED", "details", results))

	// A little lazy, but test the REST API code from here as well
	api, err := api.NewStorage(db, fs)
	require.Nil(t, err)

	apiD, err := api.DeviceGet(d.Uuid)
	require.Nil(t, err)
	tests, err := apiD.GetTests()
	require.Nil(t, err)
	require.Len(t, tests, 2)

	require.Equal(t, "test1-id", tests[0].Uuid)
	require.Equal(t, "test1", tests[0].Name)
	require.Equal(t, "intel-corei7-64-lmp-23", tests[0].TargetName)
	require.Equal(t, "PASSED", tests[0].Status)
	require.NotNil(t, tests[0].CompletedOn)
	require.Len(t, tests[0].Results, 0)

	require.Equal(t, "test2-id", tests[1].Uuid)
	require.Equal(t, "test1", tests[1].Name)
	require.Equal(t, "intel-corei7-64-lmp-23", tests[1].TargetName)
	require.Equal(t, "FAILED", tests[1].Status)
	require.NotNil(t, tests[0].CompletedOn)
	require.Len(t, tests[1].Results, 1)
	require.Equal(t, "res1", tests[1].Results[0].Name)

	require.NotNil(t, d.TestStoreArtifact("test1-id", "../artifact.txt", strings.NewReader("artifact content")))
	require.NotNil(t, d.TestStoreArtifact("test1-id-doesnot-exist", "artifact.txt", strings.NewReader("artifact content")))

	require.Nil(t, d.TestStoreArtifact("test1-id", "artifact.txt", strings.NewReader("artifact content")))
	fd, err := apiD.GetTestArtifact("test1-id", "artifact.txt")
	require.Nil(t, err)
	t.Cleanup(func() {
		require.Nil(t, fd.Close())
	})
	content, err := io.ReadAll(fd)
	require.Nil(t, err)
	require.Equal(t, "artifact content", string(content))
}
