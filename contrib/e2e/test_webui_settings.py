# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

"""Web UI tests for user settings — API token creation and audit log."""

from datetime import date, timedelta

SERVER_URL = "http://localhost:8080"


def test_settings_api_token_and_audit_log(page, update_server):
    """Create an API token via the settings dialog, verify it appears in the list,
    then confirm the audit log records the creation."""
    # Warm-up navigation: the noauth provider writes the CSRF cookie to the
    # *response* on the first ever request, but the template reads .CsrfToken
    # from the *incoming* request cookie — so the meta csrf-token tag is always
    # absent on a brand-new browser context.  Navigating to root first lets the
    # browser store the Set-Cookie, then the /settings request sends it back and
    # the server can render the meta tag (and the page's fetch() wrapper can
    # inject X-CSRF-Token automatically on subsequent AJAX calls).
    page.goto(f"{SERVER_URL}/")
    page.goto(f"{SERVER_URL}/settings")

    # Open the New Token dialog
    page.get_by_role("button", name="New Token").click()

    dialog = page.locator("#tokenModal")
    dialog.wait_for(state="visible")

    dialog.get_by_label("description").fill("e2e test")
    expires = (date.today() + timedelta(days=2)).strftime("%Y-%m-%d")
    dialog.locator("input[type='date']").fill(expires)
    dialog.locator("input[type='checkbox']").first.check()

    # Register alert handler to catch any server-side error surfaced via JS alert()
    js_dialog_message = None

    def handle_alert(js_dialog):
        nonlocal js_dialog_message
        js_dialog_message = js_dialog.message
        js_dialog.accept()

    page.on("dialog", handle_alert)
    dialog.get_by_role("button", name="Confirm").click()

    # On success the server shows #tokenCreatedModal with the new API key value.
    created_modal = page.locator("#tokenCreatedModal")
    created_modal.wait_for(state="visible")
    assert js_dialog_message is None, f"JavaScript alert: {js_dialog_message}"

    # Dismiss the modal, then reload to verify the token appears in the list
    created_modal.get_by_text("Close").click()
    page.wait_for_load_state("networkidle")
    assert "e2e test" in page.content(), "New API token not visible on settings page"

    page.get_by_text("View audit log").click()
    page.wait_for_load_state("networkidle")
    assert (
        "Token created" in page.content()
    ), f"Audit log does not show API token creation event:\n{page.content()}"
