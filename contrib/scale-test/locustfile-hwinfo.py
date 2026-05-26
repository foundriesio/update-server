"""
Locust load test for /system-info endpoint using mTLS and lshw output.

Each simulated user represents one fake device. The test reads the lshw output from each device directory and posts it to the /system-info endpoint using mTLS.

Usage:
    locust -f locustfile-hwinfo.py \
        --host https://<gateway-host>:8443 \
        --users 5000 \
        --spawn-rate 100

Optional env vars:
    DEVICE_DIR   - base directory containing device-<n>/ sub-dirs
                   (default: /data/fake-devices)
    NUM_DEVICES  - total number of devices available (default: 5000)
"""

import os
import random
import subprocess

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


class HwInfoUser(HttpUser):
    """One Locust user = one fake device posting lshw info with mTLS."""

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
        with open("/usr/share/lshw.txt", "r") as f:
            self._lshw_content = f.read()

    @tag("system-info")
    @task
    def put_system_info(self) -> None:
        """
        PUT /system_info with lshw -json output as body.
        """
        headers = {"Content-Type": "application/json"}
        device_lock = _get_device_lock(self._device_idx)
        with device_lock:
            with self.client.put(
                "/system_info",
                data=self._lshw_content,
                headers=headers,
                catch_response=True,
                name="/system-info [hwinfo]",
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
