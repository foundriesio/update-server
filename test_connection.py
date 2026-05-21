"""E2E test: fioup device registers with dg-satellite."""

import time

import pytest
import requests

SERVER_URL = "http://localhost:8080"


def _get_devices() -> list[dict]:
    """Call dg-satellite REST API and return device list."""
    resp = requests.get(f"{SERVER_URL}/v1/devices", timeout=10)
    resp.raise_for_status()
    data = resp.json()
    # API returns either a list directly or {"devices": [...]}
    if isinstance(data, list):
        return data
    return data.get("devices", data.get("items", []))


def test_device_registers(dg_satellite_server, fioup_vm):
    """fioup daemon should register the device with dg-satellite within 45s."""
    print("\n[test] Starting fioup daemon ...", flush=True)
    out, _ = fioup_vm.run("fioup check", check=False)

    try:
        devices = _get_devices()
    except requests.exceptions.RequestException as exc:
        print(f"[device] failed to checkin with: {out}", flush=True)
        pytest.fail(f"dg-satellite /v1/devices request failed: {exc}")
        return

    device = devices[0]
    last_seen = device.get("last-seen", 0)
    print(f"[test] Device found: uuid={device.get('uuid')} last-seen={last_seen}",
            flush=True)
    # last-seen is a Unix timestamp; must be non-zero to confirm contact
    assert last_seen > 0, f"Device registered but last-seen is zero: {device}"