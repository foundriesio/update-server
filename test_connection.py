"""E2E test: fioup device registers with dg-satellite."""


def test_device_registers(registered_device):
    """fioup check-in should register the device with dg-satellite."""
    last_seen = registered_device.get("last-seen", 0)
    assert last_seen > 0, f"Device registered but last-seen is zero: {registered_device}"
