# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear
"""
Locust load test for a TUF update cycle.

This workload is split into two user classes so Locust endpoint RPS remains
interpretable even when OSTree traffic dominates:
1. Control-plane users perform one control cycle per task:
   GET /repo/5.root.json (expect 404), GET /repo/targets.json,
   POST /events "EcuDownloadStarted", POST /events "EcuDownloadCompleted"
2. OSTree users continuously GET random /ostree/<path> objects

A synthetic transaction metric is emitted as:
    CYCLE update-cycle [transaction]
for each control cycle, making cycles/sec and cycle latency easy to read.

By default, this test is configured for 5000 devices.

Usage:
    locust -f locustfile-update-cycle.py \
        --host https://<gateway-host>:8443 \
        --users 1 \
        --spawn-rate 1

Optional env vars:
    DEVICE_DIR    - base directory containing device-<n>/ sub-dirs
                    (default: /data/fake-devices)
    NUM_DEVICES   - number of fake devices to include in this test
                    (default: 5000)
    OSTREE_REPO_DIR - local ostree repo root used to discover files
                      (default: /data/updates/ci/main/e2e/ostree_repo)
    DEVICE_TAG    - value sent via x-ats-tags request header
                    (default: main)
    CONTROL_USER_WEIGHT - Locust weight for control-plane users
                          (default: 1)
    OSTREE_USER_WEIGHT  - Locust weight for ostree users
                          (default: 20)
"""

import json
import os
import queue
import random
import time
import urllib.parse
import uuid

from gevent.lock import Semaphore
from locust import HttpUser, constant, events, tag, task

_DEVICE_DIR = os.environ.get("DEVICE_DIR", "/data/fake-devices")
_NUM_DEVICES = int(os.environ.get("NUM_DEVICES", "5000"))
_OSTREE_REPO_DIR = os.environ.get(
    "OSTREE_REPO_DIR", "/data/updates/ci/main/e2e/ostree_repo"
)
_DEVICE_TAG = os.environ.get("DEVICE_TAG", "main")
_CONTROL_USER_WEIGHT = int(os.environ.get("CONTROL_USER_WEIGHT", "1"))
_OSTREE_USER_WEIGHT = int(os.environ.get("OSTREE_USER_WEIGHT", "20"))

_OSTREE_PATHS: list[str] = []
_OSTREE_PATHS_GUARD = Semaphore()

_device_queue: queue.Queue[int] = queue.Queue()
for _i in range(1, _NUM_DEVICES + 1):
    _device_queue.put(_i)


def _discover_ostree_paths(repo_dir: str) -> list[str]:
    paths: list[str] = []
    for root, _dirs, files in os.walk(repo_dir):
        for name in files:
            full = os.path.join(root, name)
            rel = os.path.relpath(full, repo_dir)
            if rel == ".":
                continue
            paths.append(rel.replace(os.sep, "/"))

    if not paths:
        raise FileNotFoundError(f"No OSTree files discovered under: {repo_dir}")
    return paths


def _get_ostree_paths() -> list[str]:
    if _OSTREE_PATHS:
        return _OSTREE_PATHS

    with _OSTREE_PATHS_GUARD:
        if not _OSTREE_PATHS:
            _OSTREE_PATHS.extend(_discover_ostree_paths(_OSTREE_REPO_DIR))
    return _OSTREE_PATHS


def _build_event_payload(event_type: str, correlation_id: str, success: bool | None) -> str:
    event_data: dict[str, object] = {
        "correlationId": correlation_id,
        "ecu": "8cb909de-ff3c-4edb-9818-31687588330d",
        "targetName": "raspberrypi3-64-lmp-214",
        "version": "214",
    }
    if success is not None:
        event_data["success"] = success

    payload = [
        {
            "deviceTime": "2019-10-04T19:20:13Z",
            "event": event_data,
            "eventType": {"id": event_type, "version": 0},
            "id": str(uuid.uuid4()),
        }
    ]
    return json.dumps(payload)


class _DeviceUserBase(HttpUser):
    """Base user that binds one fake device certificate identity per user."""

    abstract = True

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
                raise FileNotFoundError(f"Missing file for device-{idx}: {path}")

        self.client.cert = (cert, key)
        self.client.verify = ca
        self._device_idx = idx
        self._ostree_paths: list[str] | None = None

    def _headers(self) -> dict[str, str]:
        return {"x-ats-tags": _DEVICE_TAG}

    def _mark_failure(self, resp, message: str) -> None:
        resp.failure(f"device-{self._device_idx} {message}")

    def _check_root_rotation(self, headers: dict[str, str]) -> bool:
        with self.client.get(
            "/repo/5.root.json",
            headers=headers,
            catch_response=True,
            name="/repo/5.root.json [expected-404]",
        ) as resp:
            if resp.status_code != 404:
                self._mark_failure(
                    resp,
                    f"expected 404, got {resp.status_code}: {resp.text}",
                )
                return False

            # Explicitly mark expected 404 as success when using catch_response.
            resp.success()
            return True

    def _fetch_targets(self, headers: dict[str, str]) -> bool:
        with self.client.get(
            "/repo/targets.json",
            headers=headers,
            catch_response=True,
            name="/repo/targets.json [control-plane]",
        ) as resp:
            if not resp.ok:
                self._mark_failure(resp, f"{resp.status_code}: {resp.text}")
                return False
            return True

    def _post_event(
        self,
        *,
        event_type: str,
        correlation_id: str,
        success: bool | None,
        request_name: str,
        headers: dict[str, str],
    ) -> bool:
        payload = _build_event_payload(
            event_type=event_type,
            correlation_id=correlation_id,
            success=success,
        )
        with self.client.post(
            "/events",
            data=payload,
            headers={"Content-Type": "application/json", **headers},
            catch_response=True,
            name=request_name,
        ) as resp:
            if not resp.ok:
                self._mark_failure(resp, f"{resp.status_code}: {resp.text}")
                return False
            return True

    def _emit_cycle_metric(self, *, started_at: float, success: bool) -> None:
        response_time_ms = (time.perf_counter() - started_at) * 1000
        events.request.fire(
            request_type="CYCLE",
            name="update-cycle [transaction]",
            response_time=response_time_ms,
            response_length=0,
            exception=None if success else RuntimeError("control cycle failed"),
            context={},
        )

    def _get_user_ostree_paths(self) -> list[str]:
        if self._ostree_paths is None:
            self._ostree_paths = _get_ostree_paths()
        return self._ostree_paths


class ControlPlaneUser(_DeviceUserBase):
    """User that executes control-plane calls once per task."""

    weight = _CONTROL_USER_WEIGHT

    @tag("update-cycle", "control-plane")
    @task
    def run_control_cycle(self) -> None:
        started_at = time.perf_counter()
        headers = self._headers()
        correlation_id = str(uuid.uuid4())

        success = True
        success = self._check_root_rotation(headers) and success
        success = self._fetch_targets(headers) and success
        success = (
            self._post_event(
                event_type="EcuDownloadStarted",
                correlation_id=correlation_id,
                success=None,
                request_name="/events [download-started]",
                headers=headers,
            )
            and success
        )
        success = (
            self._post_event(
                event_type="EcuDownloadCompleted",
                correlation_id=correlation_id,
                success=True,
                request_name="/events [download-completed]",
                headers=headers,
            )
            and success
        )

        self._emit_cycle_metric(started_at=started_at, success=success)


class OstreeUser(_DeviceUserBase):
    """User that continuously performs ostree object downloads."""

    weight = _OSTREE_USER_WEIGHT

    @tag("update-cycle", "ostree")
    @task
    def fetch_ostree_object(self) -> None:
        headers = self._headers()
        ostree_paths = self._get_user_ostree_paths()
        rel = random.choice(ostree_paths)

        with self.client.get(
            f"/ostree/{urllib.parse.quote(rel, safe='/')}",
            headers=headers,
            catch_response=True,
            name="/ostree/* [data-plane]",
        ) as resp:
            if not resp.ok:
                self._mark_failure(resp, f"{resp.status_code}: {rel}: {resp.text}")


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
        help="Number of fake devices to run update cycles for.",
    )
    parser.add_argument(
        "--ostree-repo-dir",
        env_var="OSTREE_REPO_DIR",
        default=_OSTREE_REPO_DIR,
        help="Path to local ostree repo used for random object selection.",
    )
    parser.add_argument(
        "--device-tag",
        env_var="DEVICE_TAG",
        default=_DEVICE_TAG,
        help="Value sent in x-ats-tags header during cycle requests.",
    )
    parser.add_argument(
        "--control-user-weight",
        env_var="CONTROL_USER_WEIGHT",
        default=str(_CONTROL_USER_WEIGHT),
        type=int,
        help="Locust user weight for control-plane users.",
    )
    parser.add_argument(
        "--ostree-user-weight",
        env_var="OSTREE_USER_WEIGHT",
        default=str(_OSTREE_USER_WEIGHT),
        type=int,
        help="Locust user weight for ostree users.",
    )
