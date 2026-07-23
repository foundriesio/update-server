# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

"""Local username/password login flow tests for update-server using Playwright."""

SERVER_URL = "http://localhost:8080"


def test_local_login_page_renders(page, update_server_local_auth):
    """The login page renders with the username/password form visible."""
    page.goto(f"{SERVER_URL}/devices")
    assert "Login" in page.title()
    assert page.locator("#username").is_visible()
    assert page.locator("#password").is_visible()
    assert page.get_by_role("button", name="Sign in").is_visible()


def test_local_login_invalid_credentials_shows_error(page, update_server_local_auth):
    """Submitting the wrong password redisplays the form with an error banner."""
    page.goto(f"{SERVER_URL}/devices")
    page.fill("#username", "admin")
    page.fill("#password", "wrong-password")
    page.get_by_role("button", name="Sign in").click()
    error = page.locator(".login-error")
    error.wait_for(state="visible")
    assert "Invalid username or password" in error.inner_text()


def test_local_login_valid_credentials_redirects_to_devices(page, update_server_local_auth):
    """Submitting valid credentials redirects to /devices with a session cookie set."""
    page.goto(f"{SERVER_URL}/devices")
    page.fill("#username", "admin")
    page.fill("#password", "admin")
    page.get_by_role("button", name="Sign in").click()
    page.wait_for_url(f"{SERVER_URL}/devices")
    assert page.title() == "Devices - Foundries Update Server"
    cookies = page.context.cookies()
    assert any(c["name"] == "fioserver-session" for c in cookies)
