# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

"""Admin-actor Locust User: REST API on :8080, Bearer-token auth.

Kept separate from harness.DeviceUser (mTLS on :8443, client-cert auth):
different host, different auth model, and admin traffic must never displace
device registrations by stealing virtual users from -u NUM_DEVICES.
"""

import re

import locust
from locust import HttpUser, constant, events, task

DEFAULT_ADMIN_TOKEN_FILE = "/data/auth/admin_token.txt"
DEFAULT_NUM_ADMINS = 1
DEFAULT_DEVICE_LIST_LIMIT = 100

_LINK_NEXT_RE = re.compile(r'<([^>]+)>;\s*rel="next"')


class AdminConfig:
    admin_host = None
    admin_token_file = DEFAULT_ADMIN_TOKEN_FILE
    num_admins = DEFAULT_NUM_ADMINS
    device_list_limit = DEFAULT_DEVICE_LIST_LIMIT


@events.init_command_line_parser.add_listener
def _add_admin_args(parser, **_kwargs):
    parser.add_argument(
        "--admin-host",
        env_var="ADMIN_HOST",
        default=None,
        help="Base URL for the REST/UI API (default: --host with the port "
        "swapped from 8443 to 8080).",
    )
    parser.add_argument(
        "--admin-token-file",
        env_var="ADMIN_TOKEN_FILE",
        default=DEFAULT_ADMIN_TOKEN_FILE,
        help="Path to the admin bearer token written by setup.sh.",
    )
    parser.add_argument(
        "--num-admins",
        env_var="NUM_ADMINS",
        default=str(DEFAULT_NUM_ADMINS),
        type=int,
        help="Fixed number of admin users, independent of -u/--num-devices.",
    )
    parser.add_argument(
        "--device-list-limit",
        env_var="DEVICE_LIST_LIMIT",
        default=str(DEFAULT_DEVICE_LIST_LIMIT),
        type=int,
        help="Page size for GET /v1/devices.",
    )


@events.init.add_listener
def _resolve_admin_config(environment, **_kwargs):
    opts = environment.parsed_options
    AdminConfig.admin_host = opts.admin_host or _default_admin_host(environment.host)
    AdminConfig.admin_token_file = opts.admin_token_file
    AdminConfig.num_admins = opts.num_admins
    AdminConfig.device_list_limit = opts.device_list_limit
    # fixed_count is read directly off the class by the dispatcher (see
    # dispatch.py) so it must be set here, once parsed_options is available,
    # rather than baked in at class-definition time.
    #
    # host is deliberately NOT set here: Runner.spawn_users() overwrites
    # every registered User class's `host` attribute with
    # `environment.host` (the mTLS device host, :8443) immediately before
    # each spawn — see runners.py, `user_class.host = self.environment.host`
    # — so any override made here would be silently clobbered later. Instead
    # AdminUser always issues absolute URLs (see list_devices below), which
    # bypass HttpSession.base_url entirely regardless of what `host` gets
    # set to.
    PerfAdminUser.fixed_count = AdminConfig.num_admins


def _default_admin_host(device_host: str | None) -> str | None:
    if not device_host:
        return None
    return device_host.replace(":8443", ":8080").replace("https://", "http://")


class AdminUser(HttpUser):
    abstract = True
    wait_time = constant(1)
    fixed_count = 1  # overridden in _resolve_admin_config via AdminConfig.num_admins
    host = "http://localhost:8080"  # overridden in _resolve_admin_config

    def on_start(self) -> None:
        with open(AdminConfig.admin_token_file) as f:
            self._token = f.read().strip()

    def _auth_headers(self) -> dict:
        return {"Authorization": f"Bearer {self._token}"}


class PerfAdminUser(AdminUser):
    @task
    @locust.tag("admin:list-devices")
    def list_devices(self) -> None:
        path = f"/v1/devices?limit={AdminConfig.device_list_limit}&offset=0"
        while path:
            with self.client.get(
                f"{AdminConfig.admin_host}{path}",
                headers=self._auth_headers(),
                catch_response=True,
                name="/v1/devices",
            ) as resp:
                if not resp.ok:
                    resp.failure(f"{resp.status_code}: {resp.text}")
                    return
                # The Link header's rel="next" is a relative path (see
                # setPaginationHeaders in handlers_devices.go) — it must be
                # re-anchored to admin_host on every page, not passed
                # through as-is, or it resolves against HttpSession.base_url
                # (the mTLS device host Runner.spawn_users() sets on every
                # User class) instead of the admin API.
                path = _next_page(resp.headers.get("Link"))


def _next_page(link_header: str | None) -> str | None:
    if not link_header:
        return None
    match = _LINK_NEXT_RE.search(link_header)
    return match.group(1) if match else None
