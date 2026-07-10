# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

import os
import queue

from locust import HttpUser, constant

DEVICE_DIR = os.environ.get("DEVICE_DIR", "/data/fake-devices")
CERTS_DIR = os.environ.get("CERTS_DIR", "/data/certs")
NUM_DEVICES = int(os.environ.get("NUM_DEVICES", "5000"))
DEVICE_TAG = os.environ.get("DEVICE_TAG", "main")

_device_queue: queue.Queue[int] = queue.Queue()
for _i in range(1, NUM_DEVICES + 1):
    _device_queue.put(_i)


class DeviceUser(HttpUser):
    abstract = True
    wait_time = constant(0)

    def on_start(self) -> None:
        try:
            idx = _device_queue.get_nowait()
        except queue.Empty:
            self.stop()
            return

        client_pem = f"{DEVICE_DIR}/device-{idx}/client.pem"
        root = f"{CERTS_DIR}/root.crt"
        for path in (client_pem, root):
            if not os.path.exists(path):
                raise FileNotFoundError(path)

        # Combined cert+key file — requests/urllib3 accepts a single PEM path
        self.client.cert = client_pem
        self.client.verify = root
        self._idx = idx

    def _headers(self) -> dict:
        return {
            "x-ats-tags": DEVICE_TAG,
            "x-ats-target": "perf-target-1",
            "x-ats-ostreehash": "0" * 64,
        }

    def _fail(self, resp, msg: str) -> None:
        resp.failure(f"device-{self._idx} {msg}")
