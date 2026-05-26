"""
Locust load test for /events endpoint using mTLS.

Each simulated user represents one fake device. The test posts a JSON array
of update events to /events using mTLS credentials from a fake device
sub-directory.

Per batch behavior:
- One correlationId is generated and reused across all events in that batch.
- Every event id is unique.

Usage:
    locust -f locustfile-events.py \
        --host https://<gateway-host>:8443 \
        --users 5000 \
        --spawn-rate 100

Optional env vars:
    DEVICE_DIR   - base directory containing device-<n>/ sub-dirs
                   (default: /data/fake-devices)
    NUM_DEVICES  - total number of devices available (default: 5000)
"""

import json
import os
import random
import uuid

from gevent.lock import Semaphore
from locust import HttpUser, constant, events, tag, task

_DEVICE_DIR = os.environ.get("DEVICE_DIR", "/data/fake-devices")
_NUM_DEVICES = int(os.environ.get("NUM_DEVICES", "5000"))
_DEVICE_LOCKS: dict[int, Semaphore] = {}
_DEVICE_LOCKS_GUARD = Semaphore()


def _get_device_lock(device_idx: int) -> Semaphore:
    lock = _DEVICE_LOCKS.get(device_idx)
    if lock is not None:
        return lock

    # Ensure lock creation is atomic when many users start concurrently.
    with _DEVICE_LOCKS_GUARD:
        return _DEVICE_LOCKS.setdefault(device_idx, Semaphore())


def _build_events_payload() -> str:
    correlation_id = str(uuid.uuid4())
    target_name = "raspberrypi3-64-lmp-214"
    version = "214"
    ecu_primary = "6c585ba3-4148-4b1a-a23e-d6ac02b68101"
    ecu_secondary = "8cb909de-ff3c-4edb-9818-31687588330d"

    events_batch = [
        {
            "deviceTime": "2019-10-04T19:20:11Z",
            "event": {
                "correlationId": correlation_id,
                "details": "root: 2; snapshot: 12 -> 13; targets: 12 -> 13; timestamp: 12 -> 13",
                "ecu": ecu_primary,
                "success": True,
                "targetName": target_name,
                "version": version,
            },
            "eventType": {"id": "MetadataUpdateCompleted", "version": 0},
            "id": str(uuid.uuid4()),
        },
        {
            "deviceTime": "2019-10-04T19:20:13Z",
            "event": {
                "correlationId": correlation_id,
                "ecu": ecu_secondary,
                "success": True,
                "targetName": target_name,
                "version": version,
            },
            "eventType": {"id": "EcuDownloadCompleted", "version": 0},
            "id": str(uuid.uuid4()),
        },
        {
            "deviceTime": "2019-10-04T19:20:12Z",
            "event": {
                "correlationId": correlation_id,
                "ecu": ecu_secondary,
                "targetName": target_name,
                "version": version,
            },
            "eventType": {"id": "EcuDownloadStarted", "version": 0},
            "id": str(uuid.uuid4()),
        },
        {
            "deviceTime": "2019-10-04T19:20:18Z",
            "event": {
                "correlationId": correlation_id,
                "ecu": ecu_secondary,
                "targetName": target_name,
                "version": version,
            },
            "eventType": {"id": "EcuInstallationStarted", "version": 0},
            "id": str(uuid.uuid4()),
        },
        {
            "deviceTime": "2019-10-04T19:20:59Z",
            "event": {
                "correlationId": correlation_id,
                "ecu": ecu_secondary,
                "success": True,
                "targetName": target_name,
                "version": version,
            },
            "eventType": {"id": "EcuInstallationCompleted", "version": 0},
            "id": str(uuid.uuid4()),
        },
    ]
    return json.dumps(events_batch)


class EventsUser(HttpUser):
    """One Locust user = one fake device posting event batches with mTLS."""

    wait_time = constant(0)

    def on_start(self) -> None:
        idx = random.randint(1, _NUM_DEVICES)
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

    @tag("events")
    @task
    def post_events(self) -> None:
        """
        POST /events with a batch of update events as JSON array.
        """
        headers = {"Content-Type": "application/json"}
        device_lock = _get_device_lock(self._device_idx)
        payload = _build_events_payload()

        with device_lock:
            with self.client.post(
                "/events",
                data=payload,
                headers=headers,
                catch_response=True,
                name="/events [update-events]",
            ) as resp:
                if not resp.ok:
                    resp.failure(
                        f"device-{self._device_idx} {resp.status_code}: {resp.text}"
                    )
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
