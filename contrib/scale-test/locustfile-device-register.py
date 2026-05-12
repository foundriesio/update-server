# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear
"""
Locust load test for device registration via mTLS.

Each simulated user represents one fake device. On first request the
satellite server calls DeviceCreate, which is exactly what we want to
measure.

Usage:
    locust -f locustfile.py \
        --host https://<gateway-host>:8443 \
        --users 5000 \
        --spawn-rate 100

Optional env vars:
    DEVICE_DIR   - base directory containing device-<n>/ sub-dirs
                   (default: /data/fake-devices)
    NUM_DEVICES  - total number of devices available (default: 5000)
"""

import os
import queue

from locust import HttpUser, constant, events, task


_DEVICE_DIR = os.environ.get("DEVICE_DIR", "/data/fake-devices")
_NUM_DEVICES = int(os.environ.get("NUM_DEVICES", "5000"))

# Finite queue of device indices. Once exhausted, no new users are spawned.
_device_queue: queue.Queue = queue.Queue()
for _i in range(1, _NUM_DEVICES + 1):
    _device_queue.put(_i)


class DeviceUser(HttpUser):
    """One Locust user = one fake device connecting with its own mTLS cert."""

    # After the single registration request we wait 0 s so the user finishes
    # quickly and Locust reports accurate throughput numbers.
    wait_time = constant(0)

    def on_start(self) -> None:
        try:
            idx = _device_queue.get_nowait()
        except queue.Empty:
            self.stop()
            return
        device_path = os.path.join(_DEVICE_DIR, f"device-{idx}")

        cert = os.path.join(device_path, "client.pem")
        key = os.path.join(device_path, "pkey.pem")
        ca = os.path.join(device_path, "root.crt")

        for path in (cert, key, ca):
            if not os.path.exists(path):
                raise FileNotFoundError(
                    f"Missing cert file for device-{idx}: {path}"
                )

        # Configure the underlying requests.Session with per-device mTLS creds.
        self.client.cert = (cert, key)
        self.client.verify = ca
        self._device_idx = idx

    @task
    def register(self) -> None:
        """
        GET /device triggers DeviceCreate on the server the first time a
        device connects, which is the registration we want to benchmark.
        """
        with self.client.get("/device", catch_response=True, name="/device [register]") as resp:
            if not resp.ok:
                resp.failure(f"device-{self._device_idx} {resp.status_code}: {resp.text}")
        self.stop()


@events.init_command_line_parser.add_listener
def add_custom_args(parser, **_kwargs):
    parser.add_argument(
        "--device-dir",
        env_var="DEVICE_DIR",
        default=_DEVICE_DIR,
        help="Base directory containing device-<n>/ sub-directories.",
    )
    parser.add_argument(
        "--num-devices",
        env_var="NUM_DEVICES",
        default=str(_NUM_DEVICES),
        type=int,
        help="Number of fake devices available under --device-dir.",
    )
