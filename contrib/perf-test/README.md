# perf-test — self-contained mTLS Locust performance test

Measures 5 000-device mTLS registration and steady-state gateway load against
dg-sat using a Go-generated cert fleet and a three-task Locust workload.

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

> **Warning:** `make clean` deletes `./perf-test-data` entirely. dg-sat has no
> device-key rotation — a stale DB paired with freshly generated certs of the
> same UUID will 5xx. Always run `make clean` before re-running.

## Notes

- **Spawn rate is intentionally capped at ~80/s** in headless mode. dg-sat uses
  SQLite with a single writer; a burst of new device registrations serialises
  writes and can produce transient 5xx errors that are actually lock-contention
  noise. Raising the spawn rate without checking error rates is not recommended.
- **`/repo/*` is intentionally excluded.** These endpoints return 404 until
  per-device rollout metadata is seeded; adding them now would pollute
  steady-state numbers.
- **Cert generation is fast.** The Go generator (`gen-certs`) creates 5 000
  ECDSA P-256 device certs in-process in a few seconds (no openssl forks).
- **Service ordering is enforced via compose health checks.** `dg-sat` will not
  start until `setup` completes (certs must exist before the TLS listener
  opens); `locust` will not connect until `dg-sat` is healthy.
