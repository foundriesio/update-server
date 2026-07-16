# perf-test — self-contained mTLS Locust performance test

Measures device mTLS registration, admin API, and update check/download load
against fioserver using a Go-generated cert fleet and a Locust workload split
across two actors: simulated devices (mTLS, `:8443`) and an admin client
(Bearer token, `:8080`).

## Tasks

- **Device registration** is implicit — any device's first mTLS request
  auto-registers it (`authDevice`'s `DeviceCreate`). No dedicated task; the
  ramp-up phase of any run exercises it.
- **`GET /device`, `GET /config`, `POST /events`** — steady-state device
  check-in traffic, weighted 5/2/3. Always on.
- **`GET /v1/devices`** (paginated) — admin device listing, run by a small,
  fixed-size pool of admin users (`--num-admins`, default 1) independent of
  `-u`/`--num-devices`, so it never displaces device registrations.
- **Check for update + download** (`UpdateFlow`) — an ordered sequence:
  `GET /repo/timestamp.json` → `/repo/snapshot.json` → `/repo/targets.json` →
  `POST /ostree/download-urls` → `GET /ostree/config`. Off by default
  (`--update-flow-weight 0`); see "Seeding a TUF target" below for why, and
  how to turn it on.

## Quick start

```
make run
```

Open <http://localhost:8089>, set the number of users and spawn rate, then
click **Start swarming**.

## Headless / CI

```
make headless
```

The HTML report lands at `./perf-test-data/locust-report.html` and CSV files
under the same directory.

## Scale knob

```
make run NUM_DEVICES=1000
```

## Seeding a TUF target

By default, `/repo/*` and `/ostree/*` 404/400 — a device has no update
assigned until a rollout is created for it. Pass `SEED_UPDATE=1` to `make
setup`/`make run`/`make headless` to have `gen-certs` seed one automatically:
a minimal unsigned-but-structurally-valid TUF target plus a real 256KiB
ostree object, assigned by UUID to every generated device via a synchronous
rollout — all done before `fioserver serve` ever starts, so there's no
server restart or async wait involved.

```
make setup NUM_DEVICES=10 SEED_UPDATE=1
make headless NUM_DEVICES=10 SEED_UPDATE=1 UPDATE_FLOW_WEIGHT=1
```

`UPDATE_FLOW_WEIGHT` (default 0) must be set >0 separately — seeding the
fixture and enabling the Locust task are independent knobs, so you can seed
without immediately hammering `/repo/*`/`/ostree/*` if you just want the
target to show up in the UI.

`UPDATE_TAG` (default `main`) must equal `DEVICE_TAG` (also `main` by
default) — a device's `x-ats-tags` check-in header must match the tag the
rollout was created under, or `/repo/*` 404s even with seeding on.

The server doesn't verify TUF signatures on `GET` — that's a TUF-client-side
concern the Locust harness never performs — so unsigned fixture content is
legitimate here and far simpler/faster than driving a real signed upload
through the REST API.

## Cleanup

```
make clean
```

> **Warning:** `make clean` deletes `./perf-test-data` entirely. fioserver has
> no device-key rotation — a stale DB paired with freshly generated certs of
> the same UUID will 5xx. Always run `make clean` before re-running.

## Notes

- **Spawn rate is intentionally capped at ~80/s** in headless mode. fioserver
  uses SQLite with a single writer; a burst of new device registrations
  serialises writes and can produce long-tail latency on first-contact
  registration (`/device`, `/config`, `/events`) that is actually
  lock-contention, not request failures. Raising the spawn rate without
  checking error rates is not recommended.
- **Cert generation is fast.** The Go generator (`gen-certs`) creates 5 000
  ECDSA P-256 device certs in-process in a few seconds (no openssl forks).
  Seeding a TUF target (`--seed-update`) adds negligible time — well under a
  second even at 5 000 devices.
- **Service ordering is enforced via compose health checks.** `fioserver`
  will not start until `setup` completes (certs must exist before the TLS
  listener opens); `locust` will not connect until `fioserver` is healthy.
- **Admin traffic uses absolute URLs.** Locust's runner overwrites every
  registered User class's `host` attribute with the mTLS device host right
  before each spawn, so `AdminUser` can't rely on `self.host`/`base_url` —
  `admin.py` always builds full `http://.../v1/devices` URLs instead,
  including when following pagination `Link` headers (which the server
  returns as relative paths).
