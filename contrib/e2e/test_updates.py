# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

"""E2E test: upload an OTA update to update-server via fiocli."""


def test_update_upload(fiocli, sample_update):
    """Upload the cached update artifact and verify it appears in the updates list."""
    fiocli("updates", "upload", "main", "--hardware-id=amd64-linux", "fixture-update", str(sample_update))

    out = fiocli("updates", "list")
    assert "fixture-update  main" in out, f"Uploaded update not found in 'updates list':\n{out}"
    
    out = fiocli("updates", "show", "fixture-update")
    assert "Latest target name: default-1" in out

    fiocli("updates", "delete", "fixture-update")
    out = fiocli("updates", "list")
    assert "fixture-update  main" not in out, f"Deleted update still found in 'updates list':\n{out}"
