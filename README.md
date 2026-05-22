# dg-sat-e2e

End-to-end tests for [dg-satellite](https://github.com/foundriesio/dg-satellite). The suite spins up a real `dg-satellite` server, optionally boots a Debian VM via QEMU with `fioup`, and drives the system through a Playwright browser and the `satcli` CLI.

## Prerequisites

- Python 3.11+
- Docker (used to build the tools image and run QEMU/genisoimage)
- `openssl` on the host PATH

## Quick start

```bash
# Create the virtualenv and install all Python dependencies + Playwright browser
make venv

# Download all external binaries (dg-sat, satcli, fioup.deb, composectl, Debian image)
make download

# Run the connection / server smoke tests
make run
```

`make run` is equivalent to:

```bash
.venv/bin/pytest -s -v test_connection.py
```

## Running specific test files

Activate the virtualenv first:

```bash
source .venv/bin/activate
```

Then run any test file with pytest:

| Command | What it tests |
|---|---|
| `pytest -s -v test_connection.py` | Device registration (fioup check-in) |
| `pytest -s -v test_satcli.py` | `satcli` CLI against a live server with a registered device |
| `pytest -s -v test_updates.py` | OTA update upload via `satcli` |
| `pytest -s -v test_e2e_update_flow.py` | Full update flow: register device → upload update → create rollout → install → verify |
| `pytest -s -v test_remote_actions.py` | Remote actions: run a command on a device, verify via CLI and web UI |
| `pytest -s -v test_webui.py` | Web UI smoke tests (Playwright/Chromium) |
| `pytest -s -v test_webui_settings.py` | Web UI settings: API token creation and audit log |

Run the entire suite:

```bash
pytest -s -v
```

## How it works

The `conftest.py` session fixtures handle all setup automatically when pytest starts:

1. **Preflight** — checks that `docker` and `openssl` are available.
2. **Docker image** — builds `dg-sat-e2e-tools` (Debian Trixie + QEMU + genisoimage) if it doesn't already exist.
3. **Binary downloads** — fetches `dg-sat`, `satcli`, `fioup.deb`, `composectl`, and the Debian cloud image into `.cache/` (skipped if already present).
4. **PKI** — generates a self-signed CA and device certificates under a temp directory.
5. **dg-satellite server** — launches `dg-sat` listening on `http://localhost:8080`.
6. **VM** (tests that need a device) — boots the Debian image under QEMU with cloud-init, installs `fioup`, and runs a check-in to register the device.

## Cleaning up

```bash
make clean
```

This removes `.cache/`, `__pycache__/`, and `.pytest_cache/`. The `.venv/` is left intact.
