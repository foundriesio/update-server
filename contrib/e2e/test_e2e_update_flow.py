# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

"""E2E test: full update flow — register device, upload update, create rollout,
install on device, verify events and running containers."""

UPDATE_NAME = "e2e-update"
ROLLOUT_NAME = "e2e-rollout"


def test_full_update_flow(fiocli, fiocli_tail, sample_update, registered_device, fioup_device, docker):
    uuid = registered_device["uuid"]

    # Upload the update artifact
    fiocli("updates", "upload", "--hardware-id=intel-corei7-64", "main", UPDATE_NAME, str(sample_update))

    # Create a rollout targeting this specific device
    fiocli(
        "updates", "create-rollout",
        "main", UPDATE_NAME, ROLLOUT_NAME,
        "--uuids", uuid,
    )

    # Trigger the update on the device and wait for it to complete
    fioup_device.run("fioup update")

    # Tail the rollout in a background thread before triggering the update
    stop_tail = fiocli_tail(
        #"updates", "tail", "ci", "main", UPDATE_NAME, "--rollout", ROLLOUT_NAME,
        "updates", "tail", "main", UPDATE_NAME,
    )

    # Verify update events are recorded for the device
    out = fiocli("devices", "updates", uuid)
    lines = out.splitlines()
    out = fiocli("devices", "updates", uuid, lines[1].strip())

    # Verify the shellhttpd compose app is running on the device
    docker_out, _ = docker("ps")
    assert "shellhttpd" in docker_out, f"shellhttpd container not running:\n{docker_out}"
    assert "EcuInstallationCompleted(default-1) -> Succeeded" in out, f"Update events do not show successful installation:\n{docker_out}"

    # Stop the tail and collect everything that was streamed
    out = stop_tail()
    assert '"status":"Installation completed; succeeded"' in out, f"Tail did not show successful installation event:\n{out}"
