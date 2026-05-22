"""E2E test: upload an OTA update to dg-satellite via satcli."""


def test_update_upload(satcli, sample_update):
    """Upload the cached update artifact and verify it appears in the updates list."""
    satcli("updates", "upload", "ci", "main", "fixture-update", str(sample_update))

    out = satcli("updates", "list")
    assert "ci    main  fixture-update" in out, f"Uploaded update not found in 'updates list':\n{out}"

    # There is nothing to tail, so this will exit 
    satcli("updates", "tail", "ci", "main", "fixture-update")