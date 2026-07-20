# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

"""E2E test: Sanity check that a device can connect to the update-server."""


def test_connection(registered_device, fiocli):
    print("[test_connection] registered_device:", registered_device["uuid"])
    out = fiocli("devices", "list")
    print("[fiocli]", out)

    last_seen = registered_device.get("last-seen", 0)
    assert (
        last_seen > 0
    ), f"Device registered but last-seen is zero: {registered_device}"


def test_device_register(fioup_device, fiocli):
    uuid = "11111111-1111-1111-1111-111111111111"
    fioup_device.run("rm -rf /var/sota/*")
    fioup_device.run(f"DEVICE_API=http://update-server:8080/v1/devices fioup --verbose register --apps ' ' --uuid={uuid} --factory=e2e-factory --name=e2e-test-device --tag=main --api-token doesnotmatter")

    out = fiocli("devices", "show", uuid)
    print("[fiocli]", out)
    assert "name: e2e-test-device" in out