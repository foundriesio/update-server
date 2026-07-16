# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

from locust import task

from harness import DeviceUser


class PerfUser(DeviceUser):
    @task
    def get_device(self) -> None:
        with self.client.get(
            "/device",
            headers=self._headers(),
            catch_response=True,
            name="/device",
        ) as resp:
            if not resp.ok:
                self._fail(resp, f"{resp.status_code}: {resp.text}")
