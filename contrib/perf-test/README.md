# perf-test — self-contained mTLS Locust performance test

Measures device mTLS registration and steady-state gateway load against
fioserver using a Go-generated cert fleet and a Locust workload.

## Tasks

- **Device registration** is implicit — any device's first mTLS request
  auto-registers it (`authDevice`'s `DeviceCreate`). No dedicated task; the
  ramp-up phase of any run exercises it.
- **`GET /device`** — steady-state device check-in traffic. Always on.

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
  registration (`/device`) that is actually lock-contention, not request
  failures. Raising the spawn rate without checking error rates is not
  recommended.
- **Cert generation is fast.** The Go generator (`gen-certs`) creates 5 000
  ECDSA P-256 device certs in-process in a few seconds (no openssl forks).
- **Service ordering is enforced via compose health checks.** `fioserver`
  will not start until `setup` completes (certs must exist before the TLS
  listener opens); `locust` will not connect until `fioserver` is healthy.
