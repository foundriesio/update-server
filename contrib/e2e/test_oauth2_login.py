# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

"""OAuth2 device-flow login tests against a dedicated local-auth server.

Usage::

    make build   # or just: make .cache/fioserver .cache/fiocli .cache/composectl venv
    .cache/venv/bin/pytest -s -v test_oauth2_login.py

Unlike the rest of the suite this module runs no containers — only the
fioserver/fiocli binaries and Playwright's Chromium from the venv (the
shared preflight still checks that docker and composectl are present).
"""

import base64
import queue
import re
import shutil
import subprocess
import tempfile
import threading
import time
from pathlib import Path

import pytest
import requests

# Use different ports than the standard ones to avoid conflicts
UI_PORT = 8081
GW_PORT = 8444
BASE_URL = f"http://localhost:{UI_PORT}"
FACTORY = "oauth-e2e-factory"

ADMIN = ("admin", "adminpass")
# Scopes chosen to NOT intersect the CLI's default request of
# devices:read-update,updates:read-update
LIMITED = ("limited", "limitedpass")
LIMITED_SCOPES = "users:read"


@pytest.fixture(scope="module")
def local_auth_server(fioserver_bin):
    """auth-init --local server with an admin and a scope-limited user."""
    datadir = Path(tempfile.mkdtemp(prefix="fioserver-oauth-"))

    def run(*args):
        subprocess.run(
            [str(fioserver_bin), "--datadir", str(datadir), *args],
            check=True,
            capture_output=True,
        )

    print("\n[setup] Initialising auth (local mode), PKI and TUF ...", flush=True)
    run("auth-init", "--local")
    run("pki-init", "--dnsname", "localhost")
    run("tuf-init")
    run("user-add", "--username", ADMIN[0], "--password", ADMIN[1])
    run(
        "user-add", "--username", LIMITED[0], "--password", LIMITED[1],
        "--allowedscopes", LIMITED_SCOPES,
    )

    print("[setup] Starting local-auth update-server ...", flush=True)
    log_path = datadir / "server.log"
    log_file = open(log_path, "w")
    proc = subprocess.Popen(
        [
            str(fioserver_bin), "serve", "--datadir", str(datadir),
            "--uiaddr", f":{UI_PORT}", "--gatewayaddr", f":{GW_PORT}",
        ],
        stdout=log_file,
        stderr=log_file,
    )

    deadline = time.time() + 30
    while time.time() < deadline:
        try:
            requests.get(BASE_URL, timeout=2)
            break
        except requests.exceptions.ConnectionError:
            time.sleep(1)
    else:
        proc.kill()
        log_file.close()
        print(log_path.read_text())
        raise RuntimeError("local-auth update-server did not start within 30s")

    yield datadir

    proc.terminate()
    proc.wait(timeout=10)
    log_file.close()
    shutil.rmtree(datadir, ignore_errors=True)


class _CliOutput:
    """CLI output handed from the reader thread to the test via a queue."""

    def __init__(self):
        self._queue = queue.Queue()
        self._lines = []

    def put(self, line):
        self._queue.put(line)

    def next_line(self, timeout):
        """Block for the next line; raises queue.Empty on timeout."""
        self._lines.append(self._queue.get(timeout=timeout))

    def text(self):
        try:
            while True:
                self._lines.append(self._queue.get_nowait())
        except queue.Empty:
            pass
        return "".join(self._lines)


def _start_login(fiocli_bin, home: Path, context_name: str, *extra):
    """Start `fiocli login` (device flow) and collect its output lines."""
    (home / ".config").mkdir(parents=True, exist_ok=True)
    proc = subprocess.Popen(
        [str(fiocli_bin), "login", *extra, context_name, BASE_URL],
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        env={"HOME": str(home)},
    )
    output = _CliOutput()

    def _reader():
        for line in proc.stdout:
            output.put(line)

    threading.Thread(target=_reader, daemon=True).start()
    return proc, output


def _wait_device_code(output, timeout=15):
    """Parse the verification URI and user code from the CLI output."""
    deadline = time.time() + timeout
    while True:
        text = output.text()
        visit = re.search(r"Visit: (\S+)", text)
        code = re.search(r"Enter code: (\S+)", text)
        if visit and code:
            return visit.group(1), code.group(1)
        try:
            output.next_line(timeout=max(0, deadline - time.time()))
        except queue.Empty:
            raise AssertionError(f"No device code in CLI output: {output.text()!r}")


def _pace():
    """Space out rate-limited requests.

    The server's default per-IP limit is 2 req/s with burst 2, and the
    browser, the CLI's polling, and direct API calls all share 127.0.0.1.
    Pacing to human speed lets the tests run against the stock limits
    instead of raising them in the auth config.
    """
    time.sleep(1)


def _login(page, username, password):
    """Log in through the real login form the server renders."""
    page.fill("input[name=username]", username)
    page.fill("input[name=password]", password)
    _pace()
    page.click("button[type=submit]")


def test_device_flow_authorized(local_auth_server, fiocli_bin, page):
    """Test login redirect, confirm in the UI and that the CLI token works."""
    home = local_auth_server / "cli-home-authorized"
    proc, output = _start_login(fiocli_bin, home, "oauth-e2e")
    try:
        visit_uri, user_code = _wait_device_code(output)
        activate_url = f"{visit_uri}?user_code={user_code}"

        # Unauthenticated: the activation page must demand a login first
        page.goto(activate_url)
        assert page.locator("input[name=username]").is_visible()
        _login(page, *ADMIN)

        page.goto(activate_url)
        assert page.locator("input[name=user_code]").input_value() == user_code
        _pace()
        page.click("button[type=submit]")  # Next -> confirm page
        assert user_code in page.content()
        page.fill("input[name=token_description]", "e2e device flow")
        _pace()
        page.get_by_role("button", name="Authorize").click()
        assert "Activation successful" in page.content()

        # CLI polls every 5s; give it a couple of rounds
        assert proc.wait(timeout=30) == 0, output.text()
        assert "Authorization successful" in output.text()
    finally:
        if proc.poll() is None:
            proc.kill()

    # The minted token must actually authenticate API calls
    out = subprocess.run(
        [str(fiocli_bin), "devices", "list"],
        check=True,
        capture_output=True,
        text=True,
        env={"HOME": str(home)},
    )
    assert "error" not in out.stdout.lower()

    # Redeeming the token consumed the device-auth record; the code is dead
    _pace()
    page.goto(f"{BASE_URL}/auth/confirm-activation?user_code={user_code}")
    assert "invalid user code" in page.content()


def test_device_flow_denied(local_auth_server, fiocli_bin, page):
    """Denying in the UI makes the CLI exit with access_denied."""
    home = local_auth_server / "cli-home-denied"
    proc, output = _start_login(fiocli_bin, home, "oauth-e2e-denied")
    try:
        visit_uri, user_code = _wait_device_code(output)

        page.goto(f"{visit_uri}?user_code={user_code}")
        _login(page, *ADMIN)
        _pace()
        page.goto(f"{BASE_URL}/auth/confirm-activation?user_code={user_code}")
        _pace()
        page.get_by_role("button", name="Deny").click()
        assert "Activation denied" in page.content()

        assert proc.wait(timeout=30) != 0
        assert "authorization was denied" in output.text()
    finally:
        if proc.poll() is None:
            proc.kill()


def _poll_token(device_code, timeout=15):
    """Poll the token endpoint the way a registration tool would."""
    deadline = time.time() + timeout
    while time.time() < deadline:
        _pace()
        resp = requests.post(
            f"{BASE_URL}/oauth2/token/",
            data={
                "grant_type": "urn:ietf:params:oauth:grant-type:device_code",
                "device_code": device_code,
            },
        )
        body = resp.json()
        if resp.status_code == 200:
            return body
        assert body.get("error") == "authorization_pending", body
    raise AssertionError("token was not issued within the timeout")


def test_device_flow_mock_registration(local_auth_server, page):
    """fio-device-register compatibility: the tool sends the fio-dr 3-part
    scope format with no token_expires and shows the user the verification
    URL plus a code to type in, then registers a device by POSTing a CSR
    with the minted token base64-encoded."""
    _pace()
    resp = requests.post(
        f"{BASE_URL}/oauth2/authorization/device/",
        data={"scope": f"{FACTORY}:devices:create"},
    )
    resp.raise_for_status()
    code = resp.json()

    # The user visits the bare URL from the tool's output and types the code
    page.goto(code["verification_uri"])
    _login(page, *ADMIN)
    page.goto(code["verification_uri"])
    assert page.locator("input[name=user_code]").input_value() == ""
    page.fill("input[name=user_code]", code["user_code"])
    _pace()
    page.click("button[type=submit]")  # Next -> confirm page
    assert "devices:create" in page.content()
    _pace()
    page.get_by_role("button", name="Authorize").click()
    assert "Activation successful" in page.content()

    token = _poll_token(code["device_code"])
    # The 3-part scope was converted, and the omitted token_expires
    # defaulted to a 5 minute lifetime
    assert token["scope"] == "devices:create"
    assert 0 < token["expires"] - time.time() <= 310

    uuid = "22222222-2222-2222-2222-222222222222"
    key = local_auth_server / "mock-device.key"
    csr = local_auth_server / "mock-device.csr"
    subprocess.run(
        [
            "openssl", "req", "-new", "-newkey", "ec",
            "-pkeyopt", "ec_paramgen_curve:prime256v1", "-nodes",
            "-keyout", str(key), "-out", str(csr),
            "-subj", f"/OU={FACTORY}/CN={uuid}",
        ],
        check=True,
        capture_output=True,
    )

    # fio-dr sends the bearer token base64-encoded on POST /v1/devices
    b64 = base64.b64encode(token["access_token"].encode()).decode()
    _pace()
    reg = requests.post(
        f"{BASE_URL}/v1/devices",
        headers={"Authorization": f"Bearer {b64}"},
        json={
            "uuid": uuid,
            "name": "oauth-e2e-device",
            "hardware-id": "intel-corei7-64",
            "csr": csr.read_text(),
        },
    )
    assert reg.status_code == 201, reg.text
    assert "BEGIN CERTIFICATE" in reg.json()["client.pem"]

    # The create-only token must not be able to read devices
    _pace()
    denied = requests.get(
        f"{BASE_URL}/v1/devices",
        headers={"Authorization": f"Bearer {token['access_token']}"},
    )
    assert denied.status_code == 403, denied.text

    # The device was really created: registering the same uuid conflicts
    _pace()
    dup = requests.post(
        f"{BASE_URL}/v1/devices",
        headers={"Authorization": f"Bearer {b64}"},
        json={
            "uuid": uuid,
            "name": "oauth-e2e-device",
            "hardware-id": "intel-corei7-64",
            "csr": csr.read_text(),
        },
    )
    assert dup.status_code == 409, dup.text


def test_device_flow_scope_mismatch(local_auth_server, fiocli_bin, page):
    """A user without any requested scope cannot reach the confirm page."""
    home = local_auth_server / "cli-home-scopes"
    proc, output = _start_login(fiocli_bin, home, "oauth-e2e-scopes")
    try:
        visit_uri, user_code = _wait_device_code(output)

        page.goto(f"{visit_uri}?user_code={user_code}")
        _login(page, *LIMITED)
        _pace()
        page.goto(f"{BASE_URL}/auth/confirm-activation?user_code={user_code}")
        assert "missing required scope" in page.content()
    finally:
        proc.kill()
