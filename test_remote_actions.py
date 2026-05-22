"""E2E test: remote actions — run a command on a device, verify via CLI and web UI."""

SERVER_URL = "http://localhost:8080"


def test_remote_actions(page, satcli, registered_device, fioup_vm):
    uuid = registered_device["uuid"]

    # Run a command on the device and report results back to the server
    fioup_vm.run("fioup run-and-report --name e2etest /bin/dmesg")

    # Verify the test appears in the CLI listing
    out = satcli("devices", "tests", uuid)
    assert "e2etest" in out, f"Test 'e2etest' not found in devices tests output:\n{out}"

    # Parse the test ID from the first data row (header is line 0)
    lines = [l for l in out.splitlines() if l.strip()]
    name, status, test_id, _, _ = lines[1].split()
    assert name == "e2etest", f"Expected test name 'e2etest' from {out}"
    assert status == "PASSED", f"Expected test status 'PASSED' from {out}"

    # Fetch the test detail to confirm it populated
    detail = satcli("devices", "tests", uuid, test_id)
    assert "e2etest" in detail, f"Test detail does not contain 'e2etest':\n{detail}"

    # Web UI: tests list page for the device
    page.goto(f"{SERVER_URL}/devices/{uuid}/tests")
    page.wait_for_load_state("networkidle")
    assert "e2etest" in page.content(), "Tests list page does not show test name"

    # Web UI: test detail page
    page.goto(f"{SERVER_URL}/devices/{uuid}/tests/{test_id}")
    page.wait_for_load_state("networkidle")
    assert "e2etest" in page.content(), "Test detail page does not show test name"
    assert test_id in page.content(), "Test detail page does not show test ID"

    # Test artifact download
    out = satcli("devices", "tests", uuid, test_id, "console.log")
    assert "systemd[1]: No hostname configured, using default hostname." in out, f"Expected console log not found:\n{out}"
