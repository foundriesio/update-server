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

    # Open the New token dialog
    page.get_by_role("button", name="New token", exact=True).click()

    dialog = page.locator("#tokenModal")
    dialog.wait_for(state="visible")

    dialog.get_by_label("Description", exact=True).fill("e2e test")
    expires = (date.today() + timedelta(days=2)).strftime("%Y-%m-%d")
    dialog.get_by_label("Expires on", exact=True).fill(expires)
    dialog.locator("input[type='checkbox']").first.check()

    # Register alert handler to catch any server-side error surfaced via JS alert()
    js_dialog_message = None

    def handle_alert(js_dialog):
        nonlocal js_dialog_message
        js_dialog_message = js_dialog.message
        js_dialog.accept()

    page.on("dialog", handle_alert)
    dialog.get_by_role("button", name="Create token", exact=True).click()

    # On success the server shows #tokenCreatedModal with the new API key value.
    created_modal = page.locator("#tokenCreatedModal")
    created_modal.wait_for(state="visible")
    assert js_dialog_message is None, f"JavaScript alert: {js_dialog_message}"

    # Dismiss the modal, then reload to verify the token appears in the list
    created_modal.get_by_role("button", name="Close", exact=True).click()
    page.wait_for_load_state("networkidle")
    assert "e2e test" in page.content(), "New API token not visible on settings page"

    page.set_viewport_size({"width": 320, "height": 720})
    document_metrics = page.evaluate(
        """() => ({
            scrollWidth: document.documentElement.scrollWidth,
            clientWidth: document.documentElement.clientWidth,
        })"""
    )
    assert document_metrics["scrollWidth"] <= document_metrics["clientWidth"], (
        "Settings page overflows at 320px: "
        f"scrollWidth={document_metrics['scrollWidth']}, "
        f"clientWidth={document_metrics['clientWidth']}"
    )

    token_scroll = page.locator(".settings-ledger__token-scroll")
    token_metrics = token_scroll.evaluate(
        """element => ({
            scrollWidth: element.scrollWidth,
            clientWidth: element.clientWidth,
        })"""
    )
    assert token_metrics["scrollWidth"] > token_metrics["clientWidth"], (
        "Token table is not horizontally scrollable at 320px: "
        f"scrollWidth={token_metrics['scrollWidth']}, "
        f"clientWidth={token_metrics['clientWidth']}"
    )

    page.get_by_text("View audit log").click()
    page.wait_for_load_state("networkidle")
    assert (
        "Token created" in page.content()
    ), f"Audit log does not show API token creation event:\n{page.content()}"
