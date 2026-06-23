// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/foundriesio/update-server/storage"
	"github.com/foundriesio/update-server/storage/api"
	"github.com/foundriesio/update-server/storage/gateway"
)

// dummyPubKey is a hardcoded RSA public key PEM block. It is only displayed in
// the UI and is never verified by the seed tool.
const dummyPubKey = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA2a2rwplBQLzHPZe5TNJG
O9pQBXaLqRGS4KMQpQs3wMYNg7guAlT7xHGQNpBsXhTNkqMFGbHLK3XFI+djNBKD
4nlYRjMVDMGUCVKHmXpRxMIq6N1hIBfJrAXtP9iBNV6eXB2n0j7mYwXzZvRpPoD9
7BFIL8A2RmaXYYSSGFOZBJqfIIQgIdAoaajsGfkf2JIQN0KlzJIVVgvA3JaVbG3T
LRm4kXgBiH47vkJC8M7oYpj3KZS8VaVFCpWVgkIVtMNh3qqDC9gMjOq3hQcVU6UR
YoEdwHGJ3jVQYVt5M3Z5bkqxZ0n8LxFSjuE7pqQqJKLmXZuIF1RZKoHb7pmJWxkv
LQIDAQAB
-----END PUBLIC KEY-----`

var groups = []string{"alpha", "beta", "gamma", "delta", "epsilon"}

// openStorage opens the filesystem, database, gateway, and API storage handles
// for the given datadir. It is shared by seedDevices and seedUpdates.
func openStorage(datadir string) (*storage.FsHandle, *api.Storage, *gateway.Storage, error) {
	fs, err := storage.NewFs(datadir)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load filesystem: %w", err)
	}
	db, err := storage.NewDb(fs.Config.DbFile())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load database: %w", err)
	}
	gw, err := gateway.NewStorage(db, fs)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to open gateway storage: %w", err)
	}
	ap, err := api.NewStorage(db, fs)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to open api storage: %w", err)
	}
	return fs, ap, gw, nil
}

func seedDevices(datadir string, numDevices int) error {
	_, ap, gw, err := openStorage(datadir)
	if err != nil {
		return err
	}

	created := 0
	skipped := 0

	for i := 1; i <= numDevices; i++ {
		uuid := fmt.Sprintf("seed-device-%05d", i)

		// Check if device already exists; skip creation if so.
		existing, err := gw.DeviceGet(uuid)
		if err != nil {
			return fmt.Errorf("DeviceGet(%s): %w", uuid, err)
		}
		var d *gateway.Device
		if existing != nil {
			log.Printf("skip  %s (already exists)", uuid)
			skipped++
			d = existing
		} else {
			d, err = gw.DeviceCreate(uuid, dummyPubKey)
			if err != nil {
				return fmt.Errorf("DeviceCreate(%s): %w", uuid, err)
			}
			created++
			log.Printf("create %s", uuid)
		}

		targetName := fmt.Sprintf("intel-corei7-64-lmp-%d", 100+i)
		ostreeHash := fmt.Sprintf("%064x", i*0xdeadbeef)
		if err := d.CheckIn(targetName, "main", ostreeHash, "shellhttpd,nginx"); err != nil {
			return fmt.Errorf("CheckIn(%s): %w", uuid, err)
		}

		hwInfo := fmt.Sprintf(`{"hwId":"intel-corei7-64","serial":"SN-%05d","machine":"seed"}`, i)
		if err := d.PutFile(storage.HwInfoFile, hwInfo); err != nil {
			return fmt.Errorf("PutFile(hw-info, %s): %w", uuid, err)
		}

		netInfo := fmt.Sprintf(`{"hostname":"%s","local_ipv4":"192.168.1.%d","mac":"de:ad:be:ef:00:%02x"}`,
			uuid, 100+i, i)
		if err := d.PutFile(storage.NetInfoFile, netInfo); err != nil {
			return fmt.Errorf("PutFile(net-info, %s): %w", uuid, err)
		}

		name := fmt.Sprintf("seed-device-%05d", i)
		group := groups[(i-1)%len(groups)]
		namePtr := name
		groupPtr := group
		if err := ap.PatchDeviceLabels(
			map[string]*string{"name": &namePtr, "group": &groupPtr},
			[]string{uuid},
		); err != nil {
			return fmt.Errorf("PatchDeviceLabels(%s): %w", uuid, err)
		}

		tomlConfig := fmt.Sprintf(`[device]
  uuid = "%s"
  tag = "main"

[pacman]
  type = "ostree+compose_apps"
`, uuid)
		if err := ap.SaveDeviceConfig(uuid, tomlConfig, "noauth-fake-user", "seed"); err != nil {
			return fmt.Errorf("SaveDeviceConfig(%s): %w", uuid, err)
		}
	}

	fmt.Printf("seed complete: %d created, %d skipped (total requested: %d)\n", created, skipped, numDevices)
	return nil
}

func main() {
	datadir := flag.String("datadir", "", "path to the server data directory (required)")
	numDevices := flag.Int("devices", 5, "number of devices to seed")
	numUpdates := flag.Int("updates", 2, "number of fake updates to seed")
	flag.Parse()

	if *datadir == "" {
		fmt.Fprintln(os.Stderr, "error: --datadir is required")
		flag.Usage()
		os.Exit(1)
	}

	if err := seedDevices(*datadir, *numDevices); err != nil {
		log.Fatalf("seed devices failed: %v", err)
	}

	if *numUpdates > 0 {
		fs, ap, _, err := openStorage(*datadir)
		if err != nil {
			log.Fatalf("seed updates: open storage: %v", err)
		}
		if err := seedUpdates(fs, ap, *numUpdates); err != nil {
			log.Fatalf("seed updates failed: %v", err)
		}
	}
}
