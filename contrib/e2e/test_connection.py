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
