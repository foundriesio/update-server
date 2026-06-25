# Run Locally with Mock Data

Bring the server up on `http://localhost:8080` with a populated UI, no Factory
PKI, and no real devices. Intended for local development and UI exploration only.

## Prerequisites

- **Go** — to build and run the server and seed tool
- **openssl** — to generate a throwaway self-signed certificate
- **git-lfs** — the web UI's Pico CSS asset is stored in Git LFS; run
  `git lfs pull` if the UI looks unstyled

## Quick start

```
./contrib/run-local.sh [datadir]
```

`datadir` defaults to `./.local-data`.

## What it does

- Builds `bin/fioserver` from source.
- Creates a self-signed EC P-256 certificate valid for 10 years with
  `CN=localhost`. The same certificate is reused as the device CA (`cas.pem`)
  so the gateway starts without real Factory PKI. **Dev-only — do not use in
  production.**
- Runs `auth-init --test` to configure the `noauth` provider (no login
  required) and generate an HMAC session key.
- Seeds 5 mock devices with display names, groups, hardware info, and a device
  config via `go run ./cmd/seed`.
- Starts `fioserver serve`, binding the UI on `:8080` and the device gateway
  on `:8443`.

## Browse the UI

```
http://localhost:8080/devices
```

No login is required. The `noauth` provider grants full access automatically.

## Running with login (local auth)

Pass `--auth` to start the server with the `local` username/password provider
instead of `noauth`:

```
./contrib/run-local.sh --auth [datadir]
```

This seeds an initial user (`admin` / `admin` by default) and prints the login
URL at startup. **Dev-only — do not use in production.**

Override the credentials via environment variables:

```
AUTH_USER=myuser AUTH_PASS=mypassword ./contrib/run-local.sh --auth
```

Note: re-running against an existing `datadir` will fail at `auth-init` because
the HMAC secret already exists. Remove the data directory first:

```
rm -rf ./.local-data
```

## Reset

Remove the data directory to start fresh:

```
rm -rf ./.local-data
```

## Known limitations

- **Seeded updates are synthetic.** The seed tool creates N fake updates under
  the `main` tag (default: 2, named `148`, `149`, …) with structurally valid TUF
  metadata and a sample rollout targeting the `alpha` group. The `ostree_repo/`
  and `apps/` directories are token stubs — no real device can pull from them.
  See the [updates guide](./updates.md) for adding real update content.
- **The gateway mTLS port (`:8443`) will not accept real devices.** The
  self-signed certificate is not trusted by actual FoundriesFactory devices.
  Follow the [Quick Start](./quick-start.md) guide for a production-grade PKI
  setup.
