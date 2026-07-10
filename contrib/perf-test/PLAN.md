# Implementation Plan: Self-Contained Locust mTLS Performance Test

**Target repo:** `/workspace/forks/update-server` (dg-satellite)
**Deliverable:** a new, self-contained folder `contrib/perf-test/` providing a
Locust-based mTLS performance test that registers and drives **5000 devices**.
**Status:** approved plan, ready to implement. This document is the single source of
truth — implement exactly as written.

---

## 1. Background an executor must know (verified against the code)

- **Server**: `dg-sat` (Go). Gateway = mTLS on **:8443**; UI/REST API = HTTP on **:8080**.
- **Auto-registration**: a device does **not** pre-register. On its first mTLS request the
  gateway middleware reads `cert.Subject.CommonName` as the device **UUID** and calls
  `DeviceCreate` if unknown.
  - Source: `server/gateway/middleware.go` → `authDevice` and `checkinDevice`.
- **Trust model**: the gateway trusts **any** client cert whose chain validates against a
  CA in `datadir/certs/cas.pem`. TLS is `tls.VerifyClientCertIfGiven`, min TLS 1.2.
  - Source: `server/gateway/server.go` → `loadTlsConfig`. **Certs are read ONCE at boot
    with no retry** — they must exist before `serve` starts.
- **Server TLS material** lives at: `datadir/certs/tls.pem`, `datadir/certs/tls.key`,
  `datadir/certs/cas.pem`.
- **Endpoints used by this test** (all under mTLS group, confirmed in
  `server/gateway/handlers.go`):
  - `GET /device` — server-side device info; triggers `DeviceCreate` on first contact.
  - `GET /config` — merged factory/group/device config.
  - `POST /events` — JSON array of device lifecycle events.
- **Check-in headers** (read in `checkinDevice`): `x-ats-tags`, `x-ats-target`,
  `x-ats-ostreehash`, `x-ats-dockerapps`. Sending them updates last-seen + tracked state.
- **DB**: SQLite, single writer. A 5000-device first-contact burst serialises writes and
  can emit `SQLITE_BUSY`/5xx — must be mitigated by spawn-rate throttling (see §6).
- **No device-key rotation**: if a UUID already exists in the DB with a different pubkey,
  the server errors. Therefore a **fresh data volume per run** is required (see §8 clean).

**Out of scope for now (explicit decision):** `GET /repo/*.json`. It returns 404 until
per-device rollout metadata is seeded, which would pollute steady-state numbers. Do not
add a `/repo` task.

**Reference-only (do NOT depend on at runtime; read for patterns):**
- `contrib/gen-certs.sh` — existing (slow) cert shapes: CN=UUID, OU=factory, EKUs, serial.
- `contrib/scale-test/setup.sh` — auth-init + readiness probe pattern.
- `contrib/scale-test/locustfile-update-cycle.py` — `_DeviceUserBase`, device queue,
  `_build_event_payload`, custom CLI args via `events.init_command_line_parser`.
- `contrib/scale-test/locustfile-events.py` — event batch shape.
- `/workspace/active/test-dspl/users/device.py` — the `client.cert` / `client.verify`
  mTLS pattern (the "core minimum harness" this is modelled on).

**Why a new folder & a Go generator:** `contrib/gen-certs.sh` forks `openssl` ~4× per
device (~20k process spawns for 5000 devices); even parallelised it takes minutes. The
requirement is the **fastest possible** cert generation and a **fully self-contained**
setup. A single Go binary doing all keygen+signing in-process (no forks) achieves seconds.

---

## 2. Folder layout to create

```
contrib/perf-test/
├── README.md
├── Makefile
├── docker-compose.yml
├── Dockerfile
├── requirements.txt
├── gen-certs/
│   └── main.go
├── setup.sh
├── harness.py
└── locustfile.py
```

All paths below are relative to `contrib/perf-test/` unless prefixed with the repo root.

---

## 3. `gen-certs/main.go` — fast in-process cert generator

A standalone Go `package main` using only the stdlib (`crypto/ecdsa`, `crypto/elliptic`,
`crypto/rand`, `crypto/x509`, `crypto/x509/pkix`, `encoding/pem`, `math/big`, `runtime`,
`sync`, `flag`, `os`, `path/filepath`, plus `github.com/google/uuid` **only if already in
go.mod** — otherwise generate UUIDv4 manually from `crypto/rand`, see §3.4).

### 3.1 CLI flags
- `--datadir` (string, required) — e.g. `/data`.
- `--num-devices` (int, default 5000).
- `--hostname` (string, default `dg-sat`) — the gateway host the locust container reaches;
  becomes the server cert SAN. Must equal the compose service name.
- `--factory` (string, default `dg-satellite-fake`) — device cert OU.

### 3.2 Output layout (create dirs as needed)
```
<datadir>/certs/factory_ca.pem      # CA cert (PEM)
<datadir>/certs/factory_ca.key      # CA EC private key (PEM)
<datadir>/certs/cas.pem             # == factory_ca.pem (server trusts device certs)
<datadir>/certs/tls.pem             # server gateway cert, signed by CA, SAN=hostname
<datadir>/certs/tls.key             # server gateway EC private key
<datadir>/certs/root.crt            # == factory_ca.pem (devices verify server against this)
<datadir>/fake-devices/device-<n>/client.pem   # leaf cert + EC private key in ONE file
```
`n` runs `1..num-devices`.

### 3.3 Certificate specs (match dg-sat expectations)

**Factory CA** (generate once, keep parsed `*x509.Certificate` + key in memory):
- Key: ECDSA P-256.
- Subject: `CN=Factory-CA`.
- `BasicConstraintsValid=true`, `IsCA=true`.
- `KeyUsage = x509.KeyUsageCertSign`.
- `ExtKeyUsage = {ExtKeyUsageClientAuth, ExtKeyUsageServerAuth}`.
- Validity: `NotBefore = now-5m`, `NotAfter = now + 20y`. Self-signed.

**Server TLS cert** (`tls.pem`/`tls.key`), signed by the CA:
- Key: ECDSA P-256.
- Subject: `CN=<hostname>`.
- `DNSNames = [<hostname>]` (SAN — **required** or all 5000 handshakes fail name
  verification on the client side).
- `KeyUsage = KeyUsageDigitalSignature | KeyUsageKeyEncipherment`.
- `ExtKeyUsage = {ExtKeyUsageServerAuth}`.
- Validity: `NotBefore = now-5m`, `NotAfter = now + 10y`.

**Device leaf cert** (per device), signed by the CA:
- Key: ECDSA P-256.
- Subject: `CN=<UUIDv4>`, `OU=<factory>`.
- `KeyUsage = KeyUsageDigitalSignature`.
- `ExtKeyUsage = {ExtKeyUsageClientAuth}`.
- `SerialNumber`: unique per device — use the loop index as `big.NewInt(int64(n))`, or an
  atomic counter. **Do NOT derive the serial from the UUID string** (the old Python idea
  `int("0x"+uuid,16)` crashes on hyphens — avoided here entirely).
- Validity: `NotBefore = now-5m`, `NotAfter = now + 10y`.
- Output: write leaf cert PEM block **followed by** the EC private key PEM block into the
  single file `device-<n>/client.pem`. (Order: cert first, then key. `requests`/`urllib3`
  accept a combined cert+key file for `client.cert`.)

### 3.4 UUID
Prefer `github.com/google/uuid` if it's already a dependency (`grep google/uuid go.mod`).
Otherwise generate RFC-4122 v4 manually: read 16 bytes from `crypto/rand`, set
`b[6]=(b[6]&0x0f)|0x40`, `b[8]=(b[8]&0x3f)|0x80`, format
`%08x-%04x-%04x-%04x-%012x`. Lowercase.

### 3.5 Concurrency (the speed win)
- Parse/hold the CA cert and key **once** in memory.
- Worker pool: `runtime.NumCPU()` goroutines consuming device indices from a channel; each
  goroutine generates a key, builds the template, calls `x509.CreateCertificate` against
  the shared CA, and writes its own `device-<n>/client.pem`. Use a `sync.WaitGroup`.
- Each goroutine gets its own `pkix`/template locals — no shared mutable cert state.
- `x509.CreateCertificate` with a shared parent cert+key is safe for concurrent use (it
  only reads the parent). Writes go to distinct files → no contention.
- Print a one-line timing summary at the end (e.g. `generated N devices in <duration>`).
  (Note: timing uses `time.Since`; this is a normal Go binary, not a workflow script — the
  `Date.now()` restriction does not apply here.)

### 3.6 Acceptance for this file
`go run ./gen-certs --datadir /tmp/t --num-devices 5000 --hostname dg-sat` finishes in a
few seconds and produces 5000 `client.pem` files + the 6 `certs/` files.

---

## 4. `harness.py` — trimmed DeviceUser mTLS base

Model on `test-dspl/users/device.py` + `scale-test/.../_DeviceUserBase`, minimal:

- Module-level config from env (with the locust `--x` CLI mirror in §5):
  - `DEVICE_DIR` (default `/data/fake-devices`)
  - `CERTS_DIR` (default `/data/certs`)
  - `NUM_DEVICES` (default `5000`)
  - `DEVICE_TAG` (default `main`)
- Module-level `queue.Queue[int]` pre-filled `1..NUM_DEVICES` (each device used once for
  the initial registration; tasks then repeat for that identity).
- `class DeviceUser(HttpUser)` with `abstract = False`, `wait_time = constant(0)`:
  - `on_start`:
    - `idx = queue.get_nowait()` inside try/except `queue.Empty` → `self.stop(); return`.
    - `client_pem = f"{DEVICE_DIR}/device-{idx}/client.pem"`;
      `root = f"{CERTS_DIR}/root.crt"`.
    - Assert both files exist (raise `FileNotFoundError` with the path if not).
    - `self.client.cert = client_pem` (single combined file — **not** a tuple).
    - `self.client.verify = root`.
    - store `self._idx = idx`.
  - `_headers()` → `{"x-ats-tags": DEVICE_TAG, "x-ats-target": "perf-target-1",
    "x-ats-ostreehash": "0"*64}`.
  - `_fail(resp, msg)` helper → `resp.failure(f"device-{self._idx} {msg}")`.

Keep it import-light: `os`, `queue`, `json`, `uuid`, `from locust import HttpUser,
constant`.

---

## 5. `locustfile.py` — the workload

Import `DeviceUser` base from `harness` (or define tasks directly on it). Single user
class with three `@task`-weighted methods, all using `catch_response=True`:

- `@task(5) get_device`: `GET /device` with `_headers()`. Fail unless `resp.ok`.
- `@task(2) get_config`: `GET /config` with `_headers()`. Fail unless `resp.ok`.
- `@task(3) post_events`: `POST /events` with `Content-Type: application/json`, body =
  a correlated batch built like `update-cycle.py:_build_event_payload`. Minimal batch:
  one `EcuDownloadStarted` then one `EcuDownloadCompleted` sharing a fresh
  `correlationId = str(uuid.uuid4())`; each event has a unique `id = str(uuid.uuid4())`,
  `deviceTime`, `eventType {id, version:0}`, and an `event` dict with
  `correlationId/ecu/targetName/version`. Fail unless `resp.ok`.

Add a custom-args listener (`@events.init_command_line_parser.add_listener`) exposing
`--num-devices`, `--device-dir`, `--device-tag` with `env_var=` so both env and CLI work,
mirroring `update-cycle.py`.

Host is supplied at launch (`--host https://dg-sat:8443`), not hard-coded.

---

## 6. `setup.sh` — one-time seed (bash -e)

Args: `--datadir <dir>` (default `/data`), `--num-devices <n>` (default 5000),
`--hostname <h>` (default `dg-sat`). Steps:
1. `mkdir -p "$DATADIR/auth"`.
2. `dg-sat --datadir "$DATADIR" auth-init`.
3. Write `"$DATADIR/auth/auth-config.json"` (Type `local`, `AttemptsPerSecond: 4000`,
   default scopes `users:read-update`, `devices:read-update`) — copy verbatim from
   `scale-test/setup.sh`.
4. `dg-sat --datadir "$DATADIR" user-add --username admin --password admin
   --tokenfile "$DATADIR/auth/admin_token.txt" --allowedscopes users:read-update
   devices:read-update devices:delete updates:read-update`.
5. Run the generator: `gen-certs --datadir "$DATADIR" --num-devices "$NUM_DEVICES"
   --hostname "$HOSTNAME"` (binary baked into the image at `/usr/bin/gen-certs`).
6. Readiness probe: start `dg-sat --datadir "$DATADIR" serve &`, then
   `until curl -sf http://localhost:8080/v1/devices -H "Authorization: Bearer
   $(cat "$DATADIR/auth/admin_token.txt")"; do kill -0 $PID || exit 1; sleep 0.5; done`,
   then kill the server and exit 0. (Verifies certs + auth before the real run.)

---

## 7. `Dockerfile` — multi-stage, self-contained

- **Stage 1 (builder):** `golang:1.24` (match repo's Go version — check `go.mod`).
  - Copy the repo build context.
  - `go build -o /out/dg-sat ./cmd/server`.
  - `go build -o /out/gen-certs ./contrib/perf-test/gen-certs`.
- **Stage 2 (runtime):** `python:3.12-slim`.
  - `apt-get install -y --no-install-recommends curl ca-certificates sqlite3` (sqlite3 for
    verification; curl for the readiness probe).
  - `pip install --no-cache-dir -r requirements.txt`.
  - Copy `/out/dg-sat` and `/out/gen-certs` to `/usr/bin/`.
  - Copy `contrib/perf-test/{setup.sh,harness.py,locustfile.py}` into the image
    (e.g. `/perf/`). `chmod +x setup.sh`.
  - Default workdir `/perf`.

`requirements.txt`: a single pinned `locust` (use the same version as
`contrib/scale-test` if it pins one; otherwise `locust==2.43.4`).

---

## 8. `docker-compose.yml` — ordering gates

Shared bind-mount volume `${DATA_DIR:-./perf-test-data}:/data`. Three services:

1. `setup`:
   - `build` from the Dockerfile (context = repo root, dockerfile = this Dockerfile).
   - `command: ["/perf/setup.sh","--datadir","/data","--num-devices","${NUM_DEVICES:-5000}","--hostname","dg-sat"]`.
2. `dg-sat`:
   - same image; `command: ["dg-sat","--datadir","/data","serve"]`.
   - `depends_on: { setup: { condition: service_completed_successfully } }`.
   - `healthcheck`: `curl -fk https://localhost:8443/ || exit 1` is unsuitable (mTLS).
     Instead probe the UI: `CMD curl -sf http://localhost:8080/ || exit 1` with
     `interval: 2s, retries: 30`. (UI :8080 needs no client cert.)
3. `locust`:
   - same image; `working_dir: /perf`.
   - `command: ["locust","-f","locustfile.py","--host","https://dg-sat:8443",
     "--num-devices","${NUM_DEVICES:-5000}"]` (web UI mode; no `--headless`).
   - `environment: { DEVICE_DIR: /data/fake-devices, CERTS_DIR: /data/certs,
     NUM_DEVICES: "${NUM_DEVICES:-5000}", DEVICE_TAG: main }`.
   - `depends_on: { dg-sat: { condition: service_healthy } }`.
   - `ports: ["8089:8089"]`.
   - Mount the data volume read-only is fine for locust (`:/data:ro`) since it only reads
     certs.

> **Boot-race rationale:** `loadTlsConfig` reads certs once with no retry, so `dg-sat`
> must not start before `setup` has written them — hence
> `service_completed_successfully`. `locust` must not connect before the gateway is up —
> hence `service_healthy`.

---

## 9. `Makefile`

```
NUM_DEVICES ?= 5000
DATA_DIR    ?= ./perf-test-data

run:        ## web UI at http://localhost:8089
	NUM_DEVICES=$(NUM_DEVICES) DATA_DIR=$(DATA_DIR) docker compose up --build

setup:      ## generate certs + seed only
	NUM_DEVICES=$(NUM_DEVICES) DATA_DIR=$(DATA_DIR) docker compose up --build setup

headless:   ## CSV+HTML report, throttled spawn
	NUM_DEVICES=$(NUM_DEVICES) DATA_DIR=$(DATA_DIR) docker compose run --rm \
	  -e LOCUST_EXTRA="--headless -u $(NUM_DEVICES) -r 80 -t 5m \
	  --csv /data/locust-results --html /data/locust-report.html" locust ...
```
Implementation note: the simplest robust `headless` is a dedicated compose `command`
override or a second compose file; pick whichever keeps it to one `make headless`
invocation. Spawn-rate `-r 80` is the **SQLite write-contention guard** — do not raise it
without re-checking error rates.

```
clean:      ## tear down AND delete the seeded volume (no key-rotation -> must be fresh)
	-docker compose down -v
	rm -rf $(DATA_DIR)
```
`clean` **must** delete `$(DATA_DIR)`: dg-sat has no device-key rotation, so a stale DB
paired with newly-generated certs of the same UUID would 5xx.

---

## 10. `README.md`

Cover, for a non-developer:
- One-liner: what this measures (5000-device mTLS registration + steady-state gateway load).
- **Quick start:** `make run` → open `http://localhost:8089` → set users/spawn → Start.
- **Headless/CI:** `make headless` → report at `./perf-test-data/locust-report.html`.
- **Scale knob:** `make run NUM_DEVICES=1000`.
- **Cleanup:** `make clean` (warn: deletes the seeded data volume).
- **Notes:** spawn-rate ~80/s avoids SQLite write-contention false errors; `/repo/*` is
  intentionally excluded until rollout metadata seeding is added; certs are generated
  fresh each run by the Go generator (seconds).

---

## 11. Verification checklist (run after implementing)

1. **Cert speed:** `go run ./contrib/perf-test/gen-certs --datadir /tmp/t
   --num-devices 5000 --hostname dg-sat` → completes in seconds; 5000 `client.pem` +
   `certs/{factory_ca.pem,factory_ca.key,cas.pem,tls.pem,tls.key,root.crt}` exist.
2. **Smoke:** `make setup NUM_DEVICES=10` then bring up + run 10 users headless →
   `GET /device` all 200; `sqlite3 ./perf-test-data/db.sqlite 'select count(*) from
   devices'` returns 10.
3. **mTLS enforced:** `curl -k https://localhost:8443/device` (no client cert) → 403 from
   `authDevice`. (Document: this is app-layer auth; the listener is
   `VerifyClientCertIfGiven`.)
4. **Full scale:** `make headless NUM_DEVICES=5000` → open `locust-report.html`; capture
   registration throughput, p95 latency, error rate; device count = 5000 in DB.
5. **Clean:** `make clean` removes containers and `./perf-test-data`.

---

## 12. Pitfalls (do not regress)

- Server cert **SAN must equal** the compose service name (`dg-sat`) or every handshake
  fails hostname verification. The `--hostname` flag threads this through.
- Device cert **serial must not** be derived from the dashed UUID string.
- `client.cert` is a **single combined PEM path** (cert+key), not a `(cert, key)` tuple,
  because we write them into one file. (If you instead write separate files, switch to the
  tuple form — keep the two consistent.)
- `dg-sat serve` reads TLS **once at boot** — never start it before `setup` completes.
- Always run against a **fresh** data volume (no device-key rotation in dg-sat).
- Keep spawn-rate modest (~80/s) so SQLite write serialisation doesn't masquerade as
  gateway latency/errors.
- Do **not** add a `/repo/*.json` task (404 noise without rollout seeding).
- Confirm the Go toolchain version against `go.mod` before pinning the builder image.
- Confirm `dg-sat` subcommands exist as used (`auth-init`, `user-add`, `serve`) — they
  match `contrib/scale-test/setup.sh`; if the CLI has changed, mirror that file.

---

## 13. Commit guidance

Single self-contained, conventional-commit change, e.g.:
`feat(perf-test): add self-contained Locust mTLS perf test with fast Go cert generator`
Body: why (existing openssl-fork cert gen is minutes-slow; need a fast, one-command,
self-contained 5000-device mTLS load test) + what (new `contrib/perf-test/`).
