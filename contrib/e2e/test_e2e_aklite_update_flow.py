# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

"""E2E tests: aktualizr-lite full update flow against a local update-server.

One variant per ostree pull path: libostree (built-in) and fiopull. Each
registers a real aktualizr-lite device, uploads an ostree(+app) update, creates
a rollout, drives check -> update -> simulated reboot -> run, and verifies the
installed version, the running shellhttpd app, and the server-side
EcuInstallationCompleted event.
"""

import json

from conftest import (
    AKLITE_FIOPULL_TARGET_VERSION,
    AKLITE_TARGET_VERSION,
    HARDWARE_ID,
)

UPDATE_NAME = "aklite-e2e-update"
FIOPULL_UPDATE_NAME = "aklite-e2e-fiopull-update"
ROLLOUT_NAME = "aklite-e2e-rollout"
FIOPULL_ROLLOUT_NAME = "aklite-e2e-fiopull-rollout"
TAG = "main"

# aktualizr-lite return codes (from aktualizr-lite/docker-e2e-test/e2e-test.py).
RC_OK = 0
RC_CHECKIN_UPDATE_NEW_VERSION = 16
RC_INSTALL_NEEDS_REBOOT = 100

NEED_REBOOT = "/var/run/aktualizr-session/need_reboot"


def _rc(client, cmd):
    """Run cmd in the container, returning (rc, stdout, stderr)."""
    res = client._container.exec_run(["sh", "-c", cmd], demux=True)
    out, err = res.output
    return res.exit_code, (out or b"").decode(), (err or b"").decode()


def _verify_update(fiocli, fiocli_tail, aklite_device, aklite_docker,
                   uuid: str, update_name: str, rollout_name: str,
                   version: int):
    """Create rollout, and drive check/update/reboot/run for one version."""
    fiocli(
        "updates", "create-rollout",
        TAG, update_name, rollout_name,
        "--uuids", uuid,
    )

    rc, out, err = _rc(aklite_device, "aktualizr-lite check")
    assert rc == RC_CHECKIN_UPDATE_NEW_VERSION, (
        f"check rc={rc} (expected {RC_CHECKIN_UPDATE_NEW_VERSION})\n{out}\n{err}"
    )

    stop_tail = fiocli_tail("updates", "tail", TAG, update_name)

    rc, out, err = _rc(aklite_device, f"aktualizr-lite update {version}")
    assert rc == RC_INSTALL_NEEDS_REBOOT, (
        f"update rc={rc} (expected {RC_INSTALL_NEEDS_REBOOT})\n{out}\n{err}"
    )

    aklite_device.run(f"rm -f {NEED_REBOOT}")

    rc, out, err = _rc(aklite_device, "aktualizr-lite run")
    assert rc == RC_OK, f"run rc={rc} (expected {RC_OK})\n{out}\n{err}"

    status_out, _ = aklite_device.run("aktualizr-lite status --json 1")
    status = json.loads(status_out)
    assert status["applied_target"]["version"] == version, (
        f"installed version mismatch: {status}"
    )

    docker_out, _ = aklite_docker("ps")
    assert "shellhttpd" in docker_out, f"shellhttpd not running:\n{docker_out}"

    out = fiocli("devices", "updates", uuid)
    lines = out.splitlines()
    corr_id = lines[1].strip()
    events = fiocli("devices", "updates", uuid, corr_id)
    assert "EcuInstallationCompleted" in events and "-> Succeeded" in events, (
        f"server events do not show successful installation:\n{events}"
    )

    tail_out = stop_tail()
    assert '"status":"Installation completed; succeeded"' in tail_out, (
        f"tail did not show successful installation:\n{tail_out}"
    )


def test_aklite_update_flow_libostree(
    fiocli, fiocli_tail, aklite_update, aklite_registered_device,
    aklite_device, aklite_docker,
):
    uuid = aklite_registered_device["uuid"]
    version = AKLITE_TARGET_VERSION

    fiocli(
        "updates", "upload",
        f"--hardware-id={HARDWARE_ID}",
        f"--version={version}",
        f"--name={HARDWARE_ID}-lmp-{version}",
        TAG, UPDATE_NAME, str(aklite_update),
    )

    _verify_update(fiocli, fiocli_tail, aklite_device, aklite_docker,
                   uuid, UPDATE_NAME, ROLLOUT_NAME, version)


def test_aklite_update_flow_fiopull(
    fiocli, fiocli_tail, aklite_fiopull_update, aklite_registered_device,
    aklite_device, aklite_docker,
):
    # Runs after the libostree test on the same device, so it needs a higher
    # version to be seen as an upgrade.
    uuid = aklite_registered_device["uuid"]
    version = AKLITE_FIOPULL_TARGET_VERSION

    fiocli(
        "updates", "upload",
        f"--hardware-id={HARDWARE_ID}",
        f"--version={version}",
        f"--name={HARDWARE_ID}-lmp-{version}",
        TAG, FIOPULL_UPDATE_NAME, str(aklite_fiopull_update),
    )

    aklite_device.run(
        "printf '[pacman]\\nostree_pull_tool = \"fiopull\"\\n'"
        " > /etc/sota/conf.d/z-55-fiopull.toml"
    )

    _verify_update(fiocli, fiocli_tail, aklite_device, aklite_docker,
                   uuid, FIOPULL_UPDATE_NAME, FIOPULL_ROLLOUT_NAME, version)
