"""E2E test: satcli CLI against a live dg-satellite server with a registered device."""

import os
import subprocess
from pathlib import Path

import pytest

SERVER_URL = "http://localhost:8080"


def _run_satcli(satcli_bin: Path, home: Path, *args) -> str:
    result = subprocess.run(
        [str(satcli_bin), *args],
        check=True, capture_output=True, text=True,
        env={**os.environ, "HOME": str(home)},
    )
    return result.stdout


@pytest.fixture(scope="module")
def satcli(satcli_bin, dg_satellite_server):
    """Log in and return a callable that runs satcli subcommands."""
    home = dg_satellite_server / "satcli-home"
    (home / ".config").mkdir(exist_ok=True, parents=True)
    _run_satcli(satcli_bin, home, "login", "--token", "doesnotmatter", "pytestfixture", SERVER_URL)
    return lambda *args: _run_satcli(satcli_bin, home, *args)


def test_satcli_workflow(satcli, registered_device, fioup_vm):
    device_uuid = registered_device["uuid"]

    # devices list
    out = satcli("devices", "list")
    assert device_uuid in out, f"Device UUID not found in 'devices list':\n{out}"

    # devices show
    out = satcli("devices", "show", device_uuid)
    assert device_uuid in out, f"Device UUID not found in 'devices show':\n{out}"

    # push a config entry to the device
    satcli("configs", "set", "testfile=testcontent")

    # have fioup fetch and apply the config
    fioup_vm.run("fioup config-check")

    # verify the file landed on the device with the expected content
    fioup_vm.run("test -f /run/secrets/testfile")
    content, _ = fioup_vm.run("cat /run/secrets/testfile")
    assert content.strip() == "testcontent", f"Unexpected file content: {content!r}"
