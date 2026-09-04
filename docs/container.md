# Running in a Container

This guide stands up an immediately usable update server in a Docker
container. It is the container equivalent of the [Quick Start](./quick-start.md)
evaluation flow: a self-signed PKI and the "noauth" provider. For production
concerns (TLS termination, backups, failover) see the
[production guide](./production.md).

## Quick Launch with Docker Compose

From a repository checkout,
[contrib/docker-compose.yml](../contrib/docker-compose.yml) builds the image
and runs the server with host networking, keeping its state in
`.compose-server-data/` at the repository root. Initialize that state once,
then bring the service up — all from the `contrib/` directory:

```
  cd contrib
  docker compose run --rm server --datadir=/data pki-init --dnsname <HOSTNAME> --factory <FACTORY>
  docker compose run --rm server --datadir=/data auth-init --test
  docker compose run --rm server --datadir=/data tuf-init
  docker compose up -d
```

The UI is at <http://localhost:8080/> and the device gateway listens on port
8443. The service uses host networking, so both ports listen on all host
interfaces — and with the "noauth" provider, anyone who can reach port 8080 has
full access. To keep the UI local while testing, add `--uiaddr=127.0.0.1:8080`
to the `command:` list in the compose file. See [One-Time
Setup](#one-time-setup) for what `<HOSTNAME>` and `<FACTORY>` mean, and [Run the
Server](#run-the-server) for enrolling devices. The rest of this guide performs
the same steps with plain `docker` commands and a `datadir` location of your
choosing.

## Build the Image

The repository includes a Dockerfile that produces a minimal image with the
`fioserver` binary as its entrypoint:

```
  docker build -t update-server -f contrib/server/Dockerfile .
```

> [!NOTE]
> This repository stores web UI assets in Git-LFS. If the repository was cloned
> without git-lfs, the image build fails with an error naming the stubbed asset:
> "run `git lfs pull` and rebuild". A server built outside the container from
> such a checkout refuses to start with the same message.

## One-Time Setup

All server state lives in a `datadir` on the host, mounted into the
container at `/data`. Initialize it once:

```
  mkdir -p datadir
  docker run --rm -v $PWD/datadir:/data update-server \
    --datadir=/data pki-init --dnsname <HOSTNAME> --factory <FACTORY>
  docker run --rm -v $PWD/datadir:/data update-server \
    --datadir=/data auth-init --test
  docker run --rm -v $PWD/datadir:/data update-server \
    --datadir=/data tuf-init
```

`<HOSTNAME>` is the DNS name devices will use to reach the host running the
container. For purely local testing a made-up name (e.g. `local-server`)
works — add it to `/etc/hosts` on any machine that connects.

These are the same `pki-init`, `auth-init --test`, and `tuf-init` steps
described in the [Quick Start](./quick-start.md); the notes there about
backing up `tuf/keys/root.key` and `auth/hmac.secret` apply here too.

### Using an Existing Factory PKI

If you already have a FoundriesFactory PKI, replace the `pki-init` step with the
CSR flow. Run through the container the same way:

```
  docker run --rm -v $PWD/datadir:/data update-server \
    --datadir=/data create-csr --dnsname <HOSTNAME> --factory <FACTORY>
```

Sign `datadir/certs/tls.csr` as described in [Sign the
Request](./migration.md#sign-the-request), saving the result as
`datadir/certs/tls.crt`. Next grant your Factory devices access:

```
  fioctl keys ca show --just-device-cas > datadir/certs/cas.pem
```

> [!NOTE]
> With a rootful Docker installation the files the container writes are owned by
> root, so saving `tls.crt` and `cas.pem` into `datadir/certs` may require
> `sudo`.

## Run the Server

```
  docker run -d --name update-server \
    -v $PWD/datadir:/data \
    -p 8080:8080 -p 8443:8443 \
    update-server --datadir=/data serve
```

You can browse the UI at <http://localhost:8080/>. The device gateway
listens on port 8443. Devices can now be enrolled — follow
[Enroll a Device](./quick-start.md#enroll-a-device) in the Quick Start.

> [!NOTE]
> The published ports accept connections on **all** host interfaces, not just
> localhost; with the "noauth" provider anyone who can reach port 8080 has full
> access. To keep the UI local while testing, publish it as `-p
> 127.0.0.1:8080:8080`. Port 8443 is protected by mTLS and must stay reachable
> by devices.

Manage the container with the usual commands:

```
  docker logs -f update-server
  docker stop update-server
  docker start update-server
```

## See Also

- [contrib/docker-compose.yml](../contrib/README.md) — a compose-based
  developer setup using an existing factory PKI.
- [Production guide](./production.md) — running behind a TLS-terminating
  reverse proxy.
