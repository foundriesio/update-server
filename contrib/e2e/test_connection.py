# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear


def test_connection(fioup_device):
    val = fioup_device.run("fioup version")
    print("[test_connection]", val)
