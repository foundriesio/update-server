"""Web UI tests for dg-satellite using Playwright."""

import pytest

SERVER_URL = "http://localhost:8080"

# ── Static smoke tests (server only, no VM needed) ──────────────────────────


def test_root_redirects_to_devices(page, dg_satellite_server):
    """/ redirects to /devices with the expected page title."""
    page.goto(SERVER_URL)
    assert page.url.endswith("/devices")
    assert page.title() == "Devices - Satellite Server"


def test_nav_links(page, dg_satellite_server):
    """Sub-navigation contains all expected section links."""
    page.goto(f"{SERVER_URL}/devices")
    subnav = page.locator("#subnav")
    for label in ("Devices", "Configs", "Updates", "Users"):
        assert subnav.get_by_role("link", name=label).is_visible()


def test_configs_page_loads(page, dg_satellite_server):
    page.goto(f"{SERVER_URL}/configs")
    assert page.title() == "Global Configs - Satellite Server"
    assert page.get_by_role("heading", name="Global Configs").is_visible()
    assert page.get_by_role("heading", name="Groups").is_visible()


def test_updates_page_loads(page, dg_satellite_server):
    page.goto(f"{SERVER_URL}/updates")
    assert page.title() == "Updates - Satellite Server"
    assert page.get_by_role("heading", name="Updates").is_visible()


# ── Device-dependent tests (require a registered device) ────────────────────


def test_registered_device_in_table(page, registered_device):
    """A registered device appears in the devices table."""
    page.goto(f"{SERVER_URL}/devices")
    uuid = registered_device["uuid"]
    page.wait_for_selector(f"text={uuid}")
    assert page.locator("tr", has_text=uuid).is_visible()


def test_delete_dialog(page, registered_device):
    """Clicking the trash icon opens the JS delete dialog with the device UUID."""
    page.goto(f"{SERVER_URL}/devices")
    uuid = registered_device["uuid"]
    row = page.locator("tr", has_text=uuid)
    row.locator("i.trash").click()
    dialog = page.locator("#deleteDialog")
    dialog.wait_for(state="visible")
    assert page.locator("#deleteUuid").inner_text() == uuid
