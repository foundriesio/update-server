# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

"""E2E test: PKI information via `fiocli pki show` and the /pki web page."""

SERVER_URL = "http://localhost:8080"

# update_server's pki-init runs with --factory e2e-factory and
# --dnsname update-server, so these strings must show up in the root CA,
# device CA, and gateway TLS certificate subjects respectively.
EXPECTED_SUBJECTS = ("e2e-factory-root", "e2e-factory-device-ca", "update-server")
EXPECTED_SECTIONS = ("Root CA", "Gateway TLS Certificate", "Device CA", "CA Bundles")


def test_fiocli_pki_show(fiocli, update_server):
    out = fiocli("pki", "show")

    for section in EXPECTED_SECTIONS:
        assert f"{section}:" in out, f"Missing section {section!r} in 'pki show' output:\n{out}"

    for field in ("Serial:", "Issuer:", "Subject:", "Expires:"):
        assert field in out, f"Missing {field!r} in 'pki show' output:\n{out}"

    for subject in EXPECTED_SUBJECTS:
        assert subject in out, f"Missing subject {subject!r} in 'pki show' output:\n{out}"


def test_pki_page(page, update_server):
    page.goto(f"{SERVER_URL}/pki")
    assert page.title() == "PKI - Foundries Update Server"

    for section in EXPECTED_SECTIONS:
        assert page.get_by_role("heading", name=section).is_visible()

    content = page.content()
    for subject in EXPECTED_SUBJECTS:
        assert subject in content, f"Missing subject {subject!r} on /pki page"
