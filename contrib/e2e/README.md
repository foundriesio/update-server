# End-to-end tests

This directory contains an end-to-end (e2e) test suite that exercises a real
`update-server` together with a real device client (`fioup`). Unlike the unit
tests, these tests stand up the actual server binary, register a real device,
and drive genuine update, config, and remote-action flows through both the
`fiocli` CLI and the web UI.

## What gets run

Each test session wires together three moving parts:

- **update-server** — the `fioserver` binary, built from this repo, started with
  a freshly generated PKI and a temporary data directory. It listens on
  `:8080` (web UI + user API) and `:8443` (device API).
- **fioup device** — a privileged `docker:dind` container (see
  `Dockerfile.fioup`) with the [`fioup`](https://github.com/foundriesio/fioup)
  client pre-installed. It plays the role of a real device: it checks in,
  applies configs, runs remote actions, and installs compose-app updates.
- **fiocli / Playwright** — the `fiocli` CLI (built from this repo) and a
  headless Chromium browser drive the server the way an operator would.

The container runs in its own network namespace with
`--add-host=update-server:host-gateway`, so the `update-server` DNS name inside
the container resolves back to the server running on the host.

## Requirements

The following tools must be available on the host:

- `docker` (with permission to run privileged containers)
- `openssl`
- `python3` with `venv`
- `make`, `git`, `curl`, and a Go toolchain (to build `fioserver` / `fiocli`)

`make build` provisions everything else — the `fioup-e2e` image, the server and
CLI binaries, `composectl`, a Python virtualenv, and Playwright's Chromium.

## Running the tests

From this directory:

```
make run
```

This builds any missing dependencies and then runs each test module in turn.
Build artifacts are cached under `.cache/` (which is git-ignored), so
subsequent runs only rebuild what changed.

To build the dependencies without running the tests:

```
make build
```

To run an individual test module against an already-built environment, use the
virtualenv's pytest directly:

```
.cache/venv/bin/pytest -s -v test_e2e_update_flow.py
```

To start fresh (remove all cached binaries, the venv, and the update artifact):

```
make clean
```

## The tests

| Module | What it covers |
| --- | --- |
| `test_connection.py` | Sanity: a device checks in and appears in the server; `lmp-device-register`-style registration. |
| `test_fiocli.py` | `fiocli devices list/show`; pushing a config and confirming `fioup` applies it on the device. |
| `test_remote_actions.py` | `fioup run-and-report`; verifying the result via CLI and the web UI, plus artifact download. |
| `test_updates.py` | Uploading an OTA update artifact and finding it in `updates list`. |
| `test_webui.py` | Web UI smoke tests and device-table rendering. |
| `test_webui_settings.py` | Creating an API token via the settings dialog and checking the audit log. |
| `test_e2e_update_flow.py` | Full flow: upload an update, create a rollout, install on the device, and verify events plus the running container. |

## How the fixtures fit together

The shared fixtures live in `conftest.py` and are session-scoped, so the server
and device are set up once per run:

- `update_server` — generates PKI (`add_device.sh` signs a device cert against
  the generated device CA), initializes TUF and test-mode auth, and starts
  `fioserver serve`, yielding its data directory.
- `fioup_device` / `docker` — launch the `fioup` container and wait for its
  inner `dockerd` to be ready.
- `registered_device` — copies the generated device credentials plus a
  `sota.toml` into the container and runs `fioup check` so the device shows up
  on the server.
- `fiocli` / `fiocli_tail` — a logged-in `fiocli` caller, and a variant that
  tails a command in the background.
- `sample_update` — pulls a sample compose-app (`shellhttpd`) with `composectl`
  to use as the update payload; cached across runs.
