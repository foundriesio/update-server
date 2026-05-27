# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear
"""
Locust load test for OSTree object download performance.

This test intentionally targets one device UUID. Multiple users can run in
parallel, but requests are serialized per-device to avoid overlapping calls
for the same mTLS identity.

Usage:
    locust -f locustfile-ostree.py \
        --host https://<gateway-host>:8443 \
        --users 200 \
        --spawn-rate 50

Optional env vars:
    DEVICE_DIR          - base directory containing device-<n>/ sub-dirs
                          (default: /data/fake-devices)
    OSTREE_DEVICE_NAME  - fake device directory name (default: device-1)
    OSTREE_REPO_DIR     - local seeded ostree repo root used to discover files
                          (default: /data/updates/ci/main/e2e/ostree_repo)
"""

import os
import random
import urllib.parse

from gevent.lock import Semaphore
from locust import HttpUser, constant, events, tag, task

_DEVICE_DIR = os.environ.get("DEVICE_DIR", "/data/fake-devices")
_OSTREE_DEVICE_NAME = os.environ.get("OSTREE_DEVICE_NAME", "device-1")
_OSTREE_REPO_DIR = os.environ.get(
    "OSTREE_REPO_DIR", "/data/updates/ci/main/e2e/ostree_repo"
)

_OSTREE_PATHS: list[str] = []
_OSTREE_PATHS_GUARD = Semaphore()


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


class OstreeUser(HttpUser):
    """One Locust user that fetches random files from /ostree/<path>."""

    wait_time = constant(0)

    def on_start(self) -> None:
        device_path = os.path.join(_DEVICE_DIR, _OSTREE_DEVICE_NAME)
        cert = os.path.join(device_path, "client.pem")
        key = os.path.join(device_path, "pkey.pem")
        ca = os.path.join(device_path, "root.crt")
        for path in (cert, key, ca):
            if not os.path.exists(path):
                raise FileNotFoundError(
                    f"Missing cert file for {_OSTREE_DEVICE_NAME}: {path}"
                )

        self.client.cert = (cert, key)
        self.client.verify = ca
        self._device_name = _OSTREE_DEVICE_NAME
        self._ostree_paths = _get_ostree_paths()

    @tag("ostree")
    @task
    def get_random_ostree_object(self) -> None:
        """
        GET /ostree/<path> for a random file path discovered from ostree_repo.
        """
        rel = random.choice(self._ostree_paths)
        encoded_rel = urllib.parse.quote(rel, safe="/")

        with self.client.get(
            f"/ostree/{encoded_rel}",
            headers={"x-ats-tags": "main"},
            catch_response=True,
            name="/ostree/* [object]",
        ) as resp:
            if not resp.ok:
                resp.failure(
                    f"{self._device_name} {resp.status_code}: {rel}: {resp.text}"
                )


@events.init_command_line_parser.add_listener
def add_custom_args(parser, **_kwargs):
    parser.add_argument(
        "--device-dir",
        env_var="DEVICE_DIR",
        default=_DEVICE_DIR,
        help="Base directory containing device-<n>/ sub-directories.",
    )
    parser.add_argument(
        "--ostree-device-name",
        env_var="OSTREE_DEVICE_NAME",
        default=_OSTREE_DEVICE_NAME,
        help="Fake device directory name used for mTLS credentials.",
    )
    parser.add_argument(
        "--ostree-repo-dir",
        env_var="OSTREE_REPO_DIR",
        default=_OSTREE_REPO_DIR,
        help="Path to local ostree repo used for random object selection.",
    )
