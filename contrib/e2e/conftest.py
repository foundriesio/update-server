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
import requests

REPO_ROOT = Path(__file__).parent
CACHE_DIR = REPO_ROOT / ".cache"

CONTAINER_NAME = "fioup-e2e"
SERVER_UI_PORT = 8080

SOTA_TOML = """\
[import]
tls_cacert_path = "/var/sota/root.crt"
tls_clientcert_path = "/var/sota/client.pem"
tls_pkey_path = "/var/sota/pkey.pem"

[uptane]
key_source = "file"
polling_sec = 30
repo_server = "https://update-server:8443/repo"

[provision]
primary_ecu_hardware_id = "intel-corei7-64"
server = "https://update-server:8443"

[pacman]
compose_apps_root = "/var/sota/compose-apps"
compose_apps_proxy = "https://update-server:8443/app-proxy-url"
ostree_server = "https://update-server:8443/ostree"
packages_file = "/usr/package.manifest"
reset_apps = " "
reset_apps_root = "/var/sota/reset-apps"
tags = "main"
type = "ostree+compose_apps"

[storage]
path = "/var/sota/"
type = "sqlite"

[tls]
server = "https://update-server:8443"
ca_source = "file"
cert_source = "file"
pkey_source = "file"
"""


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

    def put(self, src: Path, dst: str):
        buf = io.BytesIO()
        with tarfile.open(fileobj=buf, mode="w") as tar:
            tar.add(str(src), arcname=os.path.basename(dst))
        buf.seek(0)
        self._container.put_archive(os.path.dirname(dst) or "/", buf.getvalue())

    def put_text(self, text: str, dst: str):
        with tempfile.NamedTemporaryFile("w") as tmp:
            tmp.write(text)
            tmp.flush()
            self.put(Path(tmp.name), dst)


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


@pytest.fixture(scope="session")
def registered_device(update_server, fioup_device) -> dict:
    """Run fioup check-in and wait for the device to appear in update-server."""
    print("[setup] Copying device credentials ...", flush=True)
    fioup_device.run("mkdir -p /var/sota")
    device_dir = update_server / "device"
    fioup_device.put(device_dir / "root.crt", "/var/sota/root.crt")
    fioup_device.put(device_dir / "client.pem", "/var/sota/client.pem")
    fioup_device.put(device_dir / "pkey.pem", "/var/sota/pkey.pem")
    fioup_device.put_text(SOTA_TOML, "/var/sota/sota.toml")

    print("\n[setup] Running fioup check-in ...", flush=True)
    stdout, stderr = fioup_device.run("fioup check", check=False)

    try:
        resp = requests.get(f"http://localhost:{SERVER_UI_PORT}/v1/devices", timeout=5)
        resp.raise_for_status()
        devices = resp.json()
        if devices and devices[0].get("last-seen", 0) > 0:
            device = devices[0]
            print(f"[setup] Device registered: {device['uuid']}", flush=True)
            return device
    except requests.exceptions.RequestException as exc:
        print(f"[setup] failed to checkin with: stdout({stdout}) stderr({stderr})", flush=True)
        pytest.fail(f"update-server /v1/devices request failed: {exc}")

    raise RuntimeError(f"Device did not appear in update-server: stdout({stdout}) stderr({stderr})")


@pytest.fixture(scope="session")
def update_server(request, fioserver_bin):
    """Generate PKI, start update-server; yield datadir Path."""
    datadir = Path(tempfile.mkdtemp(prefix="fioserver-"))
    gen_pki = REPO_ROOT / "gen_pki.sh"

    print("\n[setup] Generating PKI ...", flush=True)
    result = subprocess.run(
        [
            "bash",
            str(gen_pki),
            str(datadir),
            str(fioserver_bin),
            "update-server",
            "e2e-factory",
        ],
        check=True,
        capture_output=True,
        text=True,
    )
    print(result.stdout)

    print("[setup] Initialising auth (noauth/test mode) ...", flush=True)
    subprocess.run(
        [str(fioserver_bin), "--datadir", str(datadir), "auth-init", "--test"],
        check=True,
        capture_output=True,
    )
    subprocess.run(
        [str(fioserver_bin), "--datadir", str(datadir), "tuf-init"],
        check=True,
        capture_output=True,
    )

    print("[setup] Starting update-server server ...", flush=True)
    log_path = datadir / "server.log"
    log_file = open(log_path, "w")
    proc = subprocess.Popen(
        [str(fioserver_bin), "serve", "--datadir", str(datadir)],
        stdout=log_file,
        stderr=log_file,
    )

    deadline = time.time() + 30
    while time.time() < deadline:
        try:
            requests.get(f"http://localhost:{SERVER_UI_PORT}", timeout=2)
            break
        except requests.exceptions.ConnectionError:
            time.sleep(1)
    else:
        proc.kill()
        log_file.close()
        print(log_path.read_text())
        raise RuntimeError("update-server did not start within 30s")

    print(f"[setup] update-server running (pid={proc.pid})", flush=True)

    yield datadir

    proc.terminate()
    proc.wait(timeout=10)
    log_file.close()
    if request.session.testsfailed:
        print("\n[teardown] update-server log:\n" + log_path.read_text(), flush=True)
    shutil.rmtree(datadir, ignore_errors=True)


def _run_fiocli(fiocli_bin: Path, home: Path, *args) -> str:
    try:
        result = subprocess.run(
            [str(fiocli_bin), *args],
            check=True,
            capture_output=True,
            text=True,
            env={**os.environ, "HOME": str(home)},
        )
    except subprocess.CalledProcessError as e:
        print(f"[fiocli] command failed: {' '.join(str(a) for a in args)}", flush=True)
        if e.stderr:
            print(e.stderr, flush=True)
        raise
    return result.stdout


@pytest.fixture(scope="session")
def fiocli(fiocli_bin, update_server):
    """Log in once and return a callable that runs fiocli subcommands."""
    home = update_server / "fiocli-home"
    (home / ".config").mkdir(exist_ok=True, parents=True)
    _run_fiocli(
        fiocli_bin,
        home,
        "login",
        "--token",
        "doesnotmatter",
        "pytestfixture",
        "http://localhost:8080",
    )
    return lambda *args: _run_fiocli(fiocli_bin, home, *args)
