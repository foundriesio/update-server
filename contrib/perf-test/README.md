# perf-test — self-contained mTLS Locust performance test

Measures device mTLS registration, admin API, and update check/download load
against fioserver using a Go-generated cert fleet and a Locust workload split
across two actors: simulated devices (mTLS, `:8443`) and an admin client
(Bearer token, `:8080`).

Run `make` (or `make help`) from this directory for a full list of targets
and the available `PROFILE=`/`SCENE=` names — it's generated from the
Makefile's own doc-comments and the `profiles/`/`scenes/` directories, so
it never drifts out of date with what's actually implemented.

## Getting started

**Web UI, one-shot:**

```
make run
```

Open <http://localhost:8089>. You'll be asked for two numbers before you can
click **Start swarming**:

- **Number of users** — how many of the generated devices run concurrently
  in this swarm. Can be anywhere from 1 up to `NUM_DEVICES` (the total fleet
  size `gen-certs` created, 5000 by default); each simulated device stops
  itself once the pool of pre-generated devices is exhausted.
- **Spawn rate** — how many new users (devices) Locust starts per second
  during ramp-up, until "Number of users" is reached. Keep this at or below
  80/s — see "Notes" below for why higher rates produce misleading
  latency numbers rather than real request failures.

**Headless / CI, repeatable runs against a stack you keep up:**

```
make up          # starts fioserver (and one-time setup) in the background
make headless    # runs Locust headless against it; repeat as many times as you like
```

`headless` fails fast with a clear error if `fioserver` isn't up yet, rather
than let Locust run straight into connection-refused. Named shortcuts for
common scenarios (`make locust-admin`, `make locust-update-check`, ...) skip
`headless`'s `LOCUST_ARGS=` incantation — see "Running isolated scenarios"
below. `make run` (the web UI target) is unaffected by any of this — it
still brings up the whole stack itself, one-shot.

In headless mode, "number of users" and "spawn rate" from the web UI become
`NUM_DEVICES` (`-u`) and `SPAWN_RATE` (`-r`) — both apply to every command
above, and `NUM_DEVICES` again doubles as the fleet size `gen-certs` creates
(so headless mode always runs every generated device, unlike the web UI
where you can swarm fewer than the full fleet).

`SEED_UPDATE` defaults to `1`: `make setup`/`make up`/`make run` seed a TUF
target + rollout automatically, so the check-for-update/download flow
succeeds out of the box. Pass `SEED_UPDATE=0` if you specifically want an
empty/unseeded server (e.g. to test the 404 path itself, or to skip the
extra setup work when you know you'll exclude that flow anyway — see
"Running isolated scenarios" below).

The HTML report lands at `./perf-test-data/locust-report.html` by default
(CSVs alongside it; the path follows `DATA_DIR` if you override it) after
any headless run.

**Common variables**, settable on any target (`make run NUM_DEVICES=1000`).
This is the full list; `make help` prints an abbreviated version of it:

| Variable      | Default            | Meaning                                 |
|---------------|---------------------|------------------------------------------|
| `NUM_DEVICES` | `5000`              | Simulated device fleet size             |
| `SEED_UPDATE` | `1`                 | Seed a TUF target + rollout (see below) |
| `SPAWN_RATE`  | `80`                | New devices/sec in headless mode        |
| `RUN_TIME`    | `5m`                | Headless run duration                   |
| `DATA_DIR`    | `./perf-test-data`  | Where certs, keys, and reports land     |
| `UPDATE_TAG`  | `main`              | Tag to seed the update under (see below)|
| `NUM_ADMINS`  | `1`                 | Size of the fixed admin-user pool       |

When you're done: `make clean` (see "Cleanup" below).

The rest of this document covers common task recipes, tagging/scenario
detail, TUF seeding, and troubleshooting notes — skip ahead only if
"Getting started" above doesn't cover what you need.

## Common tasks

A few concrete recipes for things you're likely to actually want to run.
All use the full default fleet (`NUM_DEVICES=5000`) and the default
`SEED_UPDATE=1` unless noted — see "Running isolated scenarios" and "TUF
target seeding" below for the underlying mechanism.

**Steady-state device traffic + admin listing, no update flow:**

```
make up
make headless LOCUST_ARGS="--exclude-tags update"
```

No positional `User` class name here (unlike the named `locust-*` targets
below) — that's deliberate: omitting it spawns *every* `User` class
(`PerfUser` + `PerfAdminUser`), so both device check-in and admin listing
run together; `--exclude-tags update` only prunes the update-flow tasks out
of `PerfUser`, leaving its steady-state tasks and all of `PerfAdminUser`
untouched.

**Full update flow at scale (check-for-update + download):**

```
make up
make locust-update
```

**Find one endpoint's max throughput:**

Isolate a single tagged task with `--tags`, then read the observed RPS and
latency percentiles off the report. Spawn rate stays capped at 80/s (see
"Notes" below) regardless of scale, so past a few hundred simulated devices
you're measuring the server's actual per-endpoint ceiling, not ramp-up
speed:

```
make up
make headless NUM_DEVICES=5000 LOCUST_ARGS="PerfUser --tags device:check-in"
```

Swap the tag for any other from "Running isolated scenarios" below
(`device:config`, `device:events`, `admin:list-devices`, `update:check`,
`update:download`) to target a different endpoint.

## Tasks

- **Device registration** is implicit — any device's first mTLS request
  auto-registers it (`authDevice`'s `DeviceCreate`). No dedicated task; the
  ramp-up phase of any run exercises it.
- **`GET /device`, `GET /config`, `POST /events`** — steady-state device
  check-in traffic, weighted 5/2/3. Tags: `device:check-in`, `device:config`,
  `device:events`.
- **`GET /v1/devices`** (paginated) — admin device listing, run by a small,
  fixed-size pool of admin users (`--num-admins`, default 1) independent of
  `-u`/`--num-devices`, so it never displaces device registrations. Tag:
  `admin:list-devices`.
- **Check for update + download** (`UpdateFlow`) — an ordered sequence:
  `GET /repo/timestamp.json` → `/repo/snapshot.json` → `/repo/targets.json` →
  `POST /ostree/download-urls` → `GET /ostree/config`. Tags: `update` (the
  whole sequence), `update:check` (the first three steps only),
  `update:download` (the last two only). Only succeeds once a rollout has
  been seeded — on by default (see "TUF target seeding" below); pass
  `SEED_UPDATE=0` and this flow will 404/400 on every step unless you also
  exclude it with `--exclude-tags update` (see "Running isolated scenarios").

## Running isolated scenarios

Every task is tagged, so you can run one scenario at a time — `ceteris
paribus`, without steady-state device/admin/update traffic all competing for
the same SQLite writer simultaneously. Locust's `--tags`/`--exclude-tags`
prune the task list *before* the run starts; pass the `User` class name(s)
too so Locust doesn't also spawn a class left with zero tasks after
filtering (it errors if it does):

```
# Only the admin device-listing endpoint
locust -f locustfile.py PerfAdminUser --tags admin:list-devices ...

# Only check-for-update (no download)
locust -f locustfile.py PerfUser --tags update:check ...

# Check-for-update + download together
locust -f locustfile.py PerfUser --tags update ...

# Steady-state device traffic only, explicitly excluding the update flow
locust -f locustfile.py PerfUser --exclude-tags update ...
```

### Named targets

The four scenarios above are also available as dedicated `make` targets, so
there's nothing to remember beyond the target name:

```
make locust-admin         NUM_DEVICES=20   # admin:list-devices only
make locust-update-check  NUM_DEVICES=20   # update:check only
make locust-update        NUM_DEVICES=20   # update:check + update:download
make locust-steady-state  NUM_DEVICES=20   # device:* only, works with or without SEED_UPDATE
```

These are thin wrappers over `headless` with `LOCUST_ARGS` preset — every
other `headless` variable (`NUM_DEVICES`, `SEED_UPDATE`, `SPAWN_RATE`,
`RUN_TIME`, ...) still applies exactly as documented elsewhere in this file.

### Profiles and scenes

**Profiles** (under `profiles/`) are named scale/timing presets
(`NUM_DEVICES`/`SPAWN_RATE`/`RUN_TIME`); **scenes** (under `scenes/`) are
named task-selection presets (`LOCUST_ARGS`, i.e. the same `--tags`/`User`-
class combinations as the named targets above). Compose either or both via
`PROFILE=`/`SCENE=` on the `headless-scenario` target:

```
make headless-scenario PROFILE=smoke SCENE=update-check
make headless-scenario PROFILE=full  SCENE=admin-only
```

Built-in profiles: `smoke` (10 devices, 10/s, 10s — fast sanity check
before committing to scale) and `full` (5000 devices, 80/s, 5m — today's
default, made explicit and reusable by name). Built-in scenes:
`update-check`, `update`, `admin-only`, `steady-state` — one per named
target above, same effect.

Any value from a profile/scene can still be overridden on the command
line, since command-line assignments always take precedence over a
`.mk` file's assignments regardless of include order:

```
make headless-scenario PROFILE=smoke SCENE=update-check NUM_DEVICES=5
```

Add a new profile or scene by dropping a `NAME.mk` file with plain
`VAR := value` assignments into `profiles/` or `scenes/` — no code changes
needed.

Through plain `make headless` (bypassing the named targets/profiles/scenes
entirely), pass selection flags via `LOCUST_ARGS` directly (the Makefile's
`headless` recipe already passes `-f locustfile.py`, `--host`, and the
other fixed flags, then appends `$(LOCUST_ARGS)` at the end — Locust
accepts positional `User` class names anywhere after the flags):

```
make headless NUM_DEVICES=20 LOCUST_ARGS="PerfUser --tags update:check"
```

Fine-grained tags: `device:check-in`, `device:config`, `device:events`,
`admin:list-devices`, `update:check`, `update:download`. Coarse tag:
`update` (covers both `update:check` and `update:download`).

## TUF target seeding

Without a rollout, `/repo/*` and `/ostree/*` 404/400 — a device has no
update assigned. `SEED_UPDATE` defaults to `1`, so `make setup`/`make run`/
`make up` have `gen-certs` seed one automatically: a minimal
unsigned-but-structurally-valid TUF target plus a real 256KiB ostree object,
assigned by UUID to every generated device via a synchronous rollout — all
done before `fioserver serve` ever starts, so there's no server restart or
async wait involved. Pass `SEED_UPDATE=0` to skip this (e.g. to test the
404 path itself, or to save the extra setup work on a run that will
`--exclude-tags update` anyway):

```
make up NUM_DEVICES=10
make headless NUM_DEVICES=10 LOCUST_ARGS="PerfUser --tags update"
```

The update flow is tagged (`update`/`update:check`/`update:download`, see
"Running isolated scenarios" above) rather than gated by its own flag —
seeding the fixture and selecting the update scenario are independent
steps, so with the default `SEED_UPDATE=1` you can run a plain `make
headless` (no `LOCUST_ARGS`) and pass `--exclude-tags update` if you'd
rather keep the update flow out of a mixed run despite the fixture being
present, or seed and run `--tags update` to exercise it directly.

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
- **Service ordering is enforced via compose health checks for `run`/`up`.**
  `fioserver` will not start until `setup` completes (certs must exist
  before the TLS listener opens). `headless`/`locust-*` instead run with
  `--no-deps` against whatever's already up (see "Getting started" above) —
  re-resolving `locust`'s full dependency chain on every run would recreate
  the already-exited `setup` container to re-verify its
  `service_completed_successfully` condition, which then fails outright
  since `auth-init`/`tuf-init` refuse to run twice against an initialized
  datadir.
- **Admin traffic uses absolute URLs.** Locust's runner overwrites every
  registered User class's `host` attribute with the mTLS device host right
  before each spawn, so `AdminUser` can't rely on `self.host`/`base_url` —
  `admin.py` always builds full `http://.../v1/devices` URLs instead,
  including when following pagination `Link` headers (which the server
  returns as relative paths).
- **`--tags`/`--exclude-tags` alone can spawn a `User` class with zero tasks
  left.** Locust prunes `.tasks` by tag but doesn't drop the `User` class
  itself from the spawn pool, so e.g. `--tags admin:list-devices` without
  also restricting to `PerfAdminUser` still tries to spawn `PerfUser`
  instances too — which then crash immediately with "No tasks defined" once
  filtering has emptied their task list. Always pass the relevant `User`
  class name(s) positionally alongside `--tags`/`--exclude-tags` (see
  "Running isolated scenarios" above).
- **`PROFILE=`/`SCENE=` only work via `Makefile`'s top-level `include`, not
  target-specific variables.** `ifneq ($(PROFILE),)` / `include
  profiles/$(PROFILE).mk` is evaluated once, at parse time, before any
  `target: VAR = value` assignment ever takes effect — so the named
  `locust-*` targets set `LOCUST_ARGS` directly rather than setting `SCENE`
  and relying on the same include mechanism. Keep this in mind if adding
  more named targets: only `PROFILE=`/`SCENE=` supplied on the actual
  command line (or via `docker compose`/`make` recursive invocation) reach
  the `include` lines.
