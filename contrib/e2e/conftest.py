# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

"""pytest fixtures for update-server + fioup e2e tests."""

import io
import json
import os
import shutil
import socket
import subprocess
import sys
import tarfile
import tempfile
import threading
import time
import urllib.request
from pathlib import Path

import docker as docker_sdk
import pytest

REPO_ROOT = Path(__file__).parent
CACHE_DIR = REPO_ROOT / ".cache"

CONTAINER_NAME = "fioup-e2e"
SERVER_UI_PORT = 8080


def _check_tools():
    missing = []
    for tool in ("docker", "openssl"):
        if not shutil.which(tool):
            missing.append(tool)
    for tool in [
        "composectl",
        "fioserver",
        "fiocli",
    ]:
        if not (CACHE_DIR / tool).exists():
            missing.append(tool)
    if missing:
        sys.exit("Missing required host tools: " + ", ".join(missing))


class DockerClient:
    """Runs commands in the target container via the docker-py SDK."""

    def __init__(self, container: "docker_sdk.models.containers.Container"):
        self._container = container

    def run(self, cmd: str, check: bool = True) -> tuple[str, str]:
        res = self._container.exec_run(["sh", "-c", cmd], demux=True)
        stdout_b, stderr_b = res.output
        stdout = stdout_b.decode() if stdout_b else ""
        stderr = stderr_b.decode() if stderr_b else ""
        if check and res.exit_code != 0:
            raise RuntimeError(
                f"Command failed (rc={res.exit_code}): {cmd!r}\n"
                f"stdout={stdout}\nstderr={stderr}"
            )
        return stdout, stderr


@pytest.fixture(scope="session", autouse=True)
def preflight():
    _check_tools()


@pytest.fixture(scope="session")
def fioserver_bin(preflight) -> Path:
    return CACHE_DIR / "fioserver"


@pytest.fixture(scope="session")
def fiocli_bin(preflight) -> Path:
    return CACHE_DIR / "fiocli"


@pytest.fixture(scope="session")
def composectl_bin(preflight) -> Path:
    return CACHE_DIR / "composectl"


@pytest.fixture(scope="session")
def fioup_device(preflight):
    """Launch the fioup target container and yield a docker-exec client.

    The container image has fioup pre-installed. It runs privileged (docker:dind)
    so fioup can manage compose apps; commands are executed via `docker exec`.
    """
    docker_client = docker_sdk.from_env()

    # Remove any stale container left over from a previous run
    try:
        docker_client.containers.get(CONTAINER_NAME).remove(force=True)
    except docker_sdk.errors.NotFound:
        pass

    print("\n[setup] Starting fioup container ...", flush=True)
    container = docker_client.containers.run(
        CONTAINER_NAME,
        detach=True,
        auto_remove=True,
        privileged=True,
        name=CONTAINER_NAME,
        # Give the container its own network namespace so the inner dind daemon
        # binds published ports (e.g. shellhttpd's 8080) in isolation instead of
        # on the host, avoiding conflicts with update-server. The "update-server"
        # DNS name still resolves to the host running the server locally.
        extra_hosts={"update-server": "host-gateway"},
    )

    client = DockerClient(container)
    try:
        # Wait for the container to be ready for `docker exec`
        deadline = time.time() + 30
        while True:
            if container.exec_run("true").exit_code == 0:
                break
            if time.time() > deadline:
                raise TimeoutError("Container did not become ready within 30s")
            time.sleep(1)

        print("[setup] Container ready", flush=True)
        yield client

    finally:
        print("\n[teardown] Stopping fioup container ...", flush=True)
        try:
            container.remove(force=True)
        except docker_sdk.errors.NotFound:
            pass