# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

"""Web UI tests for update-server using Playwright."""

import sqlite3

import pytest

SERVER_URL = "http://localhost:8080"

# ── Static smoke tests (server only, no VM needed) ──────────────────────────


def test_root_redirects_to_devices(page, update_server):
    """/ redirects to /devices with the expected page title."""
    page.goto(SERVER_URL)
    assert page.url.endswith("/devices")
    assert page.title() == "Devices - Foundries Update Server"


def test_nav_links(page, update_server):
    """Sub-navigation contains all expected section links."""
    page.goto(f"{SERVER_URL}/devices")
    subnav = page.locator("#sidenav")
    for label in ("Devices", "Configs", "Updates", "Users", "PKI"):
        assert subnav.get_by_role("link", name=label).is_visible()


def test_configs_page_loads(page, update_server):
    page.goto(f"{SERVER_URL}/configs")
    assert page.title() == "Global Configs - Foundries Update Server"
    assert page.get_by_role("heading", name="Configurations").is_visible()
    assert page.get_by_role("heading", name="Groups").is_visible()


def test_updates_page_loads(page, update_server):
    page.goto(f"{SERVER_URL}/updates")
    assert page.title() == "Updates - Foundries Update Server"
    assert page.get_by_role("heading", name="Updates").is_visible()


def test_pki_page_loads(page, update_server):
    page.goto(f"{SERVER_URL}/pki")
    assert page.title() == "PKI - Foundries Update Server"
    assert page.get_by_role("heading", name="PKI", level=1).is_visible()
    assert page.get_by_role("heading", name="Certificate Registry", level=2).is_visible()
    assert page.locator(".pki-registry__summary > div").count() == 4
    assert page.locator(".pki-registry__record").count() == 4
    assert page.locator(".pki-registry__raw").count() == 4


def test_upload_dialog_opens_and_closes(page, update_server):
    """Upload dialog opens when button is clicked and closes via close button."""
    page.goto(f"{SERVER_URL}/updates")
    dialog = page.locator("#upload-dialog")
    assert not dialog.is_visible()

    page.click("#show-upload-btn")
    assert dialog.is_visible()

    page.click("#dlg-close-btn")
    assert not dialog.is_visible()


def test_update_delete_locks_dialog_while_pending(page, update_server):
    """An in-flight update deletion cannot be repeated or dismissed."""
    update_name = "pending-delete"
    with sqlite3.connect(update_server / "db.sqlite") as db:
        db.execute(
            "INSERT INTO updates(tag, name, uploaded_at, uploaded_by) "
            "VALUES(?, ?, unixepoch('now'), ?)",
            ("main", update_name, "e2e"),
        )
    (update_server / "updates" / update_name).mkdir()

    page.goto(f"{SERVER_URL}/updates")
    page.evaluate(
        """
        updateName => {
          var realFetch = window.fetch.bind(window);
          window.__deleteRequests = 0;
          window.fetch = function(url, options) {
            if (url === '/v1/updates/' + encodeURIComponent(updateName) &&
                options && options.method === 'DELETE') {
              window.__deleteRequests += 1;
              return new Promise(function(resolve) {
                window.setTimeout(function() {
                  resolve(new Response('test failure', { status: 500 }));
                }, 1000);
              });
            }
            return realFetch(url, options);
          };
        };
        """,
        update_name,
    )

    page.get_by_role("button", name=f"Delete update {update_name}").click()
    page.locator("#confirm-cb").check()
    page.locator("#deleteConfirmBtn").evaluate(
        "button => { button.click(); button.click(); }"
    )
    page.keyboard.press("Escape")
    pending_state = page.evaluate(
        """
        ({
          requests: window.__deleteRequests,
          dialogOpen: document.getElementById('deleteDialog').open,
          dialogBusy: document.getElementById('deleteDialog').getAttribute('aria-busy'),
          checkboxDisabled: document.getElementById('confirm-cb').disabled,
          deleteDisabled: document.getElementById('deleteConfirmBtn').disabled,
          closeDisabled: document.getElementById('delete-close-btn').disabled,
          cancelDisabled: document.getElementById('delete-cancel-btn').disabled
        })
        """
    )
    page.wait_for_function(
        "document.getElementById('deleteDialog').getAttribute('aria-busy') === 'false'"
    )

    assert pending_state == {
        "requests": 1,
        "dialogOpen": True,
        "dialogBusy": "true",
        "checkboxDisabled": True,
        "deleteDisabled": True,
        "closeDisabled": True,
        "cancelDisabled": True,
    }
    assert page.locator("#deleteDialog").is_visible()
    assert page.locator("#delete-status").inner_text() == (
        "Error deleting update: test failure"
    )
    assert not page.locator("#confirm-cb").is_disabled()
    assert not page.locator("#deleteConfirmBtn").is_disabled()
    assert not page.locator("#delete-close-btn").is_disabled()
    assert not page.locator("#delete-cancel-btn").is_disabled()
    assert page.locator("#delete-status").get_attribute("role") == "alert"
    assert page.evaluate("document.activeElement.id") == "delete-status"

    # A retry clears the previous error while the new request is in flight.
    page.locator("#deleteConfirmBtn").click()
    page.wait_for_function(
        "document.getElementById('deleteDialog').getAttribute('aria-busy') === 'true'"
    )
    assert page.locator("#delete-status").inner_text() == ""


def test_upload_validation_stays_inline(page, update_server):
    """Upload validation reports errors in the dialog instead of blocking alerts."""
    page.goto(f"{SERVER_URL}/updates")
    page.click("#show-upload-btn")

    page.fill("#upload-tag", "   ")
    page.fill("#upload-name", "   ")
    page.click("#upload-btn")
    assert page.evaluate("document.getElementById('upload-form').checkValidity()") is False

    page.fill("#upload-tag", "main")
    page.fill("#upload-name", "inline-validation")
    page.click("#upload-btn")
    result = page.locator("#upload-result")
    assert result.get_attribute("role") == "alert"
    assert result.inner_text() == "Please select a directory."
    assert page.evaluate("document.activeElement.id") == "upload-dir-btn"


def test_delete_dialog_contains_scroll(page, update_server):
    """Delete dialog scroll gestures do not reach the page behind it."""
    page.goto(f"{SERVER_URL}/updates")
    page.evaluate("document.getElementById('deleteDialog').showModal()")

    styles = page.locator("#deleteDialog").evaluate(
        """
        dialog => ({
          dialogOverscroll: getComputedStyle(dialog).overscrollBehavior,
          backdropOverflow: getComputedStyle(dialog, '::backdrop').overflow,
          backdropOverscroll: getComputedStyle(dialog, '::backdrop').overscrollBehavior
        })
        """
    )

    assert styles == {
        "dialogOverscroll": "contain",
        "backdropOverflow": "hidden",
        "backdropOverscroll": "contain",
    }


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
