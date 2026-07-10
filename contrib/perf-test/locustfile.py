# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

import json
import uuid

from locust import SequentialTaskSet, events, task

from admin import PerfAdminUser  # noqa: F401  (imported for its Locust User side effects)
from harness import DeviceUser

DEFAULT_UPDATE_FLOW_WEIGHT = 0


class UpdateFlowConfig:
    weight = DEFAULT_UPDATE_FLOW_WEIGHT


@events.init_command_line_parser.add_listener
def _add_update_flow_args(parser, **_kwargs):
    parser.add_argument(
        "--update-flow-weight",
        env_var="UPDATE_FLOW_WEIGHT",
        default=str(DEFAULT_UPDATE_FLOW_WEIGHT),
        type=int,
        help="Relative weight of the check-for-update/download task sequence. "
        "0 (default) disables it entirely — only enable this once the fixture "
        "has seeded a rollout for these devices (see gen-certs --seed-update), "
        "or every run will 404/400 on /repo/* and /ostree/*.",
    )


@events.init.add_listener
def _resolve_update_flow_config(environment, **_kwargs):
    opts = environment.parsed_options
    UpdateFlowConfig.weight = opts.update_flow_weight
    # PerfUser.tasks is built once, at class-definition time, by TaskSet's
    # metaclass (see locust/user/task.py get_tasks_from_base_classes) — it
    # can't read a CLI flag that hasn't been parsed yet. So the weighted
    # {callable: weight} mapping declared on the class is fixed at import
    # time; to make the weight configurable we rebuild .tasks here, once
    # parsed_options is available, using the same {task: weight} expansion
    # the metaclass would have done.
    tasks = [PerfUser.get_device] * 5 + [PerfUser.get_config] * 2 + [PerfUser.post_events] * 3
    tasks += [UpdateFlow] * UpdateFlowConfig.weight
    PerfUser.tasks = tasks


class UpdateFlow(SequentialTaskSet):
    """Ordered check-for-update + download flow.

    Modeled on a real TUF client: timestamp -> snapshot -> targets, then
    resolve the target name/hash and fetch it over /ostree/*. State resolved
    by one step (e.g. the target name parsed from targets.json) is flow-scoped
    on this TaskSet instance, not the User or a module global, so it doesn't
    leak across unrelated tasks or other users.
    """

    def on_start(self) -> None:
        self._target_name = None
        self._ostree_hash = None

    @task
    def get_timestamp(self) -> None:
        self._get_role("timestamp.json")

    @task
    def get_snapshot(self) -> None:
        self._get_role("snapshot.json")

    @task
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
    # .tasks is rebuilt in _resolve_update_flow_config() once CLI args are
    # parsed (see that function's docstring/comment for why); the methods
    # below are defined directly rather than via @task since the decorator's
    # weights would otherwise be baked in at class-definition time.
    def get_device(self) -> None:
        with self.client.get(
            "/device",
            headers=self._headers(),
            catch_response=True,
            name="/device",
        ) as resp:
            if not resp.ok:
                self._fail(resp, f"{resp.status_code}: {resp.text}")

    def get_config(self) -> None:
        with self.client.get(
            "/config",
            headers=self._headers(),
            catch_response=True,
            name="/config",
        ) as resp:
            if not resp.ok:
                self._fail(resp, f"{resp.status_code}: {resp.text}")

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
