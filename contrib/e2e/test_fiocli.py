# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

"""E2E test: fiocli CLI against a live update-server with a registered device."""


def test_fiocli_workflow(fiocli, registered_device, fioup_device):
    device_uuid = registered_device["uuid"]

    # devices list
    out = fiocli("devices", "list")
    assert device_uuid in out, f"Device UUID not found in 'devices list':\n{out}"

    # devices show
    out = fiocli("devices", "show", device_uuid)
    assert device_uuid in out, f"Device UUID not found in 'devices show':\n{out}"

    # push a config entry to the device
    fiocli("configs", "set", "testfile=testcontent")

    # have fioup fetch and apply the config
    fioup_device.run("fioup config-check")

    # verify the file landed on the device with the expected content
    fioup_device.run("test -f /run/secrets/testfile")
    content, _ = fioup_device.run("cat /run/secrets/testfile")
    assert content.strip() == "testcontent", f"Unexpected file content: {content!r}"
