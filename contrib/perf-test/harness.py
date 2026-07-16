# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

import os
import queue

from locust import HttpUser, constant, events

DEFAULT_DEVICE_DIR = "/data/fake-devices"
DEFAULT_CERTS_DIR = "/data/certs"
DEFAULT_NUM_DEVICES = 5000
DEFAULT_DEVICE_TAG = "main"

_device_queue: "queue.Queue[int]" = queue.Queue()


class DeviceConfig:
    """Resolved from CLI args/env vars in the init event, before any User spawns."""

    device_dir = DEFAULT_DEVICE_DIR
    certs_dir = DEFAULT_CERTS_DIR
    num_devices = DEFAULT_NUM_DEVICES
    device_tag = DEFAULT_DEVICE_TAG


@events.init_command_line_parser.add_listener
def _add_device_args(parser, **_kwargs):
    parser.add_argument(
        "--num-devices",
        env_var="NUM_DEVICES",
        default=str(DEFAULT_NUM_DEVICES),
        type=int,
        help="Number of fake devices available.",
    )
    parser.add_argument(
        "--device-dir",
        env_var="DEVICE_DIR",
        default=DEFAULT_DEVICE_DIR,
        help="Base directory containing device-<n>/ sub-directories.",
    )
    parser.add_argument(
        "--certs-dir",
        env_var="CERTS_DIR",
        default=DEFAULT_CERTS_DIR,
        help="Directory containing root.crt.",
    )
    parser.add_argument(
        "--device-tag",
        env_var="DEVICE_TAG",
        default=DEFAULT_DEVICE_TAG,
        help="Value sent in the x-ats-tags header.",
    )


@events.init.add_listener
def _resolve_device_config(environment, **_kwargs):
    # This must run at the init event, not at import time: Locust only
    # finishes parsing argv (and merging in env_var= defaults) right before
    # firing init, so anything read at module import sees stale/default
    # values instead of what the user actually passed.
    opts = environment.parsed_options
    DeviceConfig.device_dir = opts.device_dir
    DeviceConfig.certs_dir = opts.certs_dir
    DeviceConfig.num_devices = opts.num_devices
    DeviceConfig.device_tag = opts.device_tag
    for i in range(1, DeviceConfig.num_devices + 1):
        _device_queue.put(i)


class DeviceUser(HttpUser):
    abstract = True
    wait_time = constant(0)

    def on_start(self) -> None:
        try:
            idx = _device_queue.get_nowait()
        except queue.Empty:
            self.stop()
            return

        client_pem = f"{DeviceConfig.device_dir}/device-{idx}/client.pem"
        root = f"{DeviceConfig.certs_dir}/root.crt"
        for path in (client_pem, root):
            if not os.path.exists(path):
                raise FileNotFoundError(path)

        # Combined cert+key file — requests/urllib3 accepts a single PEM path
        self.client.cert = client_pem
        self.client.verify = root
        self._idx = idx

    def _headers(self) -> dict:
        return {
            "x-ats-tags": DeviceConfig.device_tag,
            "x-ats-target": "perf-target-1",
            "x-ats-ostreehash": "0" * 64,
        }

    def _fail(self, resp, msg: str) -> None:
        resp.failure(f"device-{self._idx} {msg}")
