# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

import json
import uuid

import locust
from locust import SequentialTaskSet, task

from admin import PerfAdminUser  # noqa: F401  (imported for its Locust User side effects)
from harness import DeviceUser


@locust.tag("update")
class UpdateFlow(SequentialTaskSet):
    """Ordered check-for-update + download flow.

    Modeled on a real TUF client: timestamp -> snapshot -> targets, then
    resolve the target name/hash and fetch it over /ostree/*. State resolved
    by one step (e.g. the target name parsed from targets.json) is flow-scoped
    on this TaskSet instance, not the User or a module global, so it doesn't
    leak across unrelated tasks or other users.

    Only meaningful once a rollout has been seeded for these devices (see
    gen-certs --seed-update) — otherwise every step 404s/400s. Run in
    isolation with --tags update:check (timestamp/snapshot/targets only) or
    --tags update (the full check+download sequence), or keep it out of an
    unseeded run entirely with --exclude-tags update.
    """

    def on_start(self) -> None:
        self._target_name = None
        self._ostree_hash = None

    @task
    @locust.tag("update:check")
    def get_timestamp(self) -> None:
        self._get_role("timestamp.json")

    @task
    @locust.tag("update:check")
    def get_snapshot(self) -> None:
        self._get_role("snapshot.json")

    @task
    @locust.tag("update:check")
    def get_targets(self) -> None:
        body = self._get_role("targets.json")
        if body is None:
            return
        try:
            targets = json.loads(body).get("signed", {}).get("targets", {})
        except (json.JSONDecodeError, AttributeError):
            return
        for name, meta in targets.items():
            sha256 = meta.get("hashes", {}).get("sha256")
            if sha256:
                self._target_name = name
                self._ostree_hash = sha256
                break

    @task
    @locust.tag("update:download")
    def request_download_urls(self) -> None:
        with self.user.client.post(
            "/ostree/download-urls",
            headers=self.user._headers(self._target_name, self._ostree_hash),
            catch_response=True,
            name="/ostree/download-urls",
        ) as resp:
            if resp.status_code != 204:
                self.user._fail(resp, f"download-urls {resp.status_code}: {resp.text}")

    @task
    @locust.tag("update:download")
    def download_ostree_config(self) -> None:
        self._get_ostree("config")

    def _get_role(self, filename: str) -> str | None:
        with self.user.client.get(
            f"/repo/{filename}",
            headers=self.user._headers(self._target_name, self._ostree_hash),
            catch_response=True,
            name=f"/repo/{filename}",
        ) as resp:
            if not resp.ok:
                self.user._fail(resp, f"{resp.status_code}: {resp.text}")
                return None
            return resp.text

    def _get_ostree(self, path: str) -> None:
        with self.user.client.get(
            f"/ostree/{path}",
            headers=self.user._headers(self._target_name, self._ostree_hash),
            catch_response=True,
            name=f"/ostree/{path}",
        ) as resp:
            if not resp.ok:
                self.user._fail(resp, f"{resp.status_code}: {resp.text}")


class PerfUser(DeviceUser):
    @locust.tag("device:check-in")
    def get_device(self) -> None:
        with self.client.get(
            "/device",
            headers=self._headers(),
            catch_response=True,
            name="/device",
        ) as resp:
            if not resp.ok:
                self._fail(resp, f"{resp.status_code}: {resp.text}")

    @locust.tag("device:config")
    def get_config(self) -> None:
        with self.client.get(
            "/config",
            headers=self._headers(),
            catch_response=True,
            name="/config",
        ) as resp:
            if not resp.ok:
                self._fail(resp, f"{resp.status_code}: {resp.text}")

    @locust.tag("device:events")
    def post_events(self) -> None:
        correlation_id = str(uuid.uuid4())
        payload = json.dumps(
            [
                {
                    "id": str(uuid.uuid4()),
                    "deviceTime": "2019-10-04T19:20:12Z",
                    "eventType": {"id": "EcuDownloadStarted", "version": 0},
                    "event": {
                        "correlationId": correlation_id,
                        "ecu": "8cb909de-ff3c-4edb-9818-31687588330d",
                        "targetName": "perf-target-lmp-1",
                        "version": "1",
                    },
                },
                {
                    "id": str(uuid.uuid4()),
                    "deviceTime": "2019-10-04T19:20:13Z",
                    "eventType": {"id": "EcuDownloadCompleted", "version": 0},
                    "event": {
                        "correlationId": correlation_id,
                        "ecu": "8cb909de-ff3c-4edb-9818-31687588330d",
                        "targetName": "perf-target-lmp-1",
                        "version": "1",
                    },
                },
            ]
        )
        with self.client.post(
            "/events",
            data=payload,
            headers={"Content-Type": "application/json", **self._headers()},
            catch_response=True,
            name="/events",
        ) as resp:
            if not resp.ok:
                self._fail(resp, f"{resp.status_code}: {resp.text}")

    # Explicit weighted mapping instead of @task(n) decorators: get_tasks_from_
    # base_classes() (locust/user/task.py) populates .tasks from *either* an
    # explicit `tasks = {...}` dict *or* @task-decorated methods found by
    # scanning class_dict — using both on the same methods double-counts them.
    # @locust.tag() above has no such conflict; it only sets .locust_tag_set.
    tasks = {
        get_device: 5,
        get_config: 2,
        post_events: 3,
        UpdateFlow: 1,
    }

