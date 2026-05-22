"""E2E test: full update flow — register device, upload update, create rollout,
install on device, verify events and running containers."""

UPDATE_NAME = "e2e-update"
ROLLOUT_NAME = "e2e-rollout"


def test_full_update_flow(satcli, sample_update, registered_device, fioup_vm):
    uuid = registered_device["uuid"]

    # Upload the update artifact
    satcli("updates", "upload", "ci", "main", UPDATE_NAME, str(sample_update))

    # Create a rollout targeting this specific device
    satcli(
        "updates", "create-rollout",
        "ci", "main", UPDATE_NAME, ROLLOUT_NAME,
        "--uuids", uuid,
    )

    # Trigger the update on the device and wait for it to complete
    fioup_vm.run("fioup update")

    # Verify update events are recorded for the device
    out = satcli("devices", "updates", uuid)
    lines = out.splitlines()
    out = satcli("devices", "updates", uuid, lines[1].strip())

    # Verify the shellhttpd compose app is running on the device
    docker_out, _ = fioup_vm.run("docker ps")
    assert "shellhttpd" in docker_out, f"shellhttpd container not running:\n{docker_out}"

    assert "EcuInstallationCompleted(intel-corei7-64-lmp-149) -> Succeeded" in out, "Update evens do not show successful installation" 
