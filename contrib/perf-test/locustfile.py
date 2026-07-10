# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

import json
import uuid

from locust import events, task

from harness import DEVICE_DIR, DEVICE_TAG, NUM_DEVICES, DeviceUser


class PerfUser(DeviceUser):
    @task(5)
    def get_device(self) -> None:
        with self.client.get(
            "/device",
            headers=self._headers(),
            catch_response=True,
            name="/device",
        ) as resp:
            if not resp.ok:
                self._fail(resp, f"{resp.status_code}: {resp.text}")

    @task(2)
    def get_config(self) -> None:
        with self.client.get(
            "/config",
            headers=self._headers(),
            catch_response=True,
            name="/config",
        ) as resp:
            if not resp.ok:
                self._fail(resp, f"{resp.status_code}: {resp.text}")

    @task(3)
    def post_events(self) -> None:
        correlation_id = str(uuid.uuid4())
        payload = json.dumps(
            [
                {
                    "id": str(uuid.uuid4()),
                    "deviceTime": "2019-10-04T19:20:12Z",
                    "eventType": {"id": "EcuDownloadStarted", "version": 0},
                    "event": {
                        "correlationId": correlation_id,
                        "ecu": "8cb909de-ff3c-4edb-9818-31687588330d",
                        "targetName": "perf-target-lmp-1",
                        "version": "1",
                    },
                },
                {
                    "id": str(uuid.uuid4()),
                    "deviceTime": "2019-10-04T19:20:13Z",
                    "eventType": {"id": "EcuDownloadCompleted", "version": 0},
                    "event": {
                        "correlationId": correlation_id,
                        "ecu": "8cb909de-ff3c-4edb-9818-31687588330d",
                        "targetName": "perf-target-lmp-1",
                        "version": "1",
                    },
                },
            ]
        )
        with self.client.post(
            "/events",
            data=payload,
            headers={"Content-Type": "application/json", **self._headers()},
            catch_response=True,
            name="/events",
        ) as resp:
            if not resp.ok:
                self._fail(resp, f"{resp.status_code}: {resp.text}")


@events.init_command_line_parser.add_listener
def add_custom_args(parser, **_kwargs):
    parser.add_argument(
        "--num-devices",
        env_var="NUM_DEVICES",
        default=str(NUM_DEVICES),
        type=int,
        help="Number of fake devices available.",
    )
    parser.add_argument(
        "--device-dir",
        env_var="DEVICE_DIR",
        default=DEVICE_DIR,
        help="Base directory containing device-<n>/ sub-directories.",
    )
    parser.add_argument(
        "--device-tag",
        env_var="DEVICE_TAG",
        default=DEVICE_TAG,
        help="Value sent in x-ats-tags header.",
    )
