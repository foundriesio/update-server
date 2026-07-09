# Quick Start

## Prerequisites

Devices authenticate using mTLS and your FoundriesFactory® PKI. You will
need access to your [Factory CA](https://docs.foundries.io/latest/reference-manual/security/device-gateway.html)
in order to create a TLS certificate for device-facing APIs.

## Install
Download the latest update server from:

 <https://github.com/foundriesio/update-server/releases>

Save as `fioserver`.
For Linux and Mac, make sure to `chmod +x fioserver`.

## Configure Mutual TLS

### Creat Certificate Signing requests for TLS

Devices need to trust the TLS connection they make to this server. In
order to do this, you must create a CSR to be signed with the Factory
root key:

```
  ./fioserver --datadir=./datadir create-csr --dnsname <HOSTNAME> --factory <FACTORY>
```

### Sign the Request

Copy `datadir/certs/tls.csr` to the computer with your factory PKI. This
file does not contain sensitive information, so it is safe to share as
needed. From the factory PKI directory run:

```
  fioctl keys ca sign --pki-dir <path to your factory pki> <path to tls.csr>
```

This command will print the contents of the certificate. The contents are
not sensitive. Go back to the update server system and create the
file `datadir/certs/tls.pem` with this content.

### Grant Access to Devices

This service needs to know what devices can connect to it. You can allow
all valid factory devices to connect with:
```
 fioctl keys ca show --just-device-cas > datadir/certs/cas.pem
```

## Configure User Authentication

The update server includes a few [authentication providers](../auth)
for user-facing APIs. The "noauth" provider is handy for starting up a
quick local environment for testing and evaluation. Running
`auth-init --test` will setup an HMAC encryption key for API
tokens and web sessions, as well as the "noauth" provider.

```
  ./fioserver --datadir=./datadir auth-init --test
```

## Initialize TUF

Before the server can sign and manage TUF metadata, the TUF keys and
root metadata must be initialized:

```
  ./fioserver --datadir=./datadir tuf-init
```

### Importing an existing fleet's TUF root

If you already have a fleet of devices provisioned against a Foundries.io
factory, initializing a brand new TUF root would leave those devices unable
to validate metadata from this server. Instead, you can import the factory's
existing root of trust so that already-provisioned devices continue to trust
updates.

The import reads every version of the factory's `root.json` from a tarball you
provide, stores them so devices can walk the trust chain, and then generates a
new root (version N+1) with fresh online keys for the `root`, `targets`,
`snapshot`, and `timestamp` roles. The new root is signed by both the factory's
offline root key (proving continuity of trust) and the new root key.

You will need:

* The factory's offline keys tarball (typically `offline-creds.tgz`),
  which contains the offline root key used to sign the rotation.
* A gzipped tarball containing all of the factory's `root.json` files (e.g.
  `1.root.json`, `2.root.json`, ...). See `fioctl keys tuf show-root`.
```
  ./fioserver --datadir=./datadir tuf-init \
    --import-keys ./offline-creds.tgz \
    --import-roots ./roots.tgz
```

Options:

* `--import-keys` — path to the fioctl offline keys tarball. Providing this
  (or `--import-roots`) enables import mode.
* `--import-roots` — path to a gzipped tarball containing all of the factory's
  `root.json` files. See `fioctl keys tuf download-roots`.

> **Note:** `tuf-init` requires `auth-init` to have been run first so that the
> imported role keys can be encrypted at rest.

> **IMPORTANT:** A successful import generates a new root key at
> `<datadir>/tufrepo/keys/root.key`. This key is encrypted at rest using the
> HMAC secret at `<datadir>/auth/hmac.secret`, so you must back up BOTH files —
> the `root.key` is useless without the `hmac.secret` needed to decrypt it.
> Store copies of both somewhere safe immediately. If either file is lost it
> CANNOT be recovered, and you will permanently lose the ability to rotate or
> manage your TUF root of trust.

## Run the Server

`./fioserv serve --datadir=datadir`

You can browse the UI at <http://localhost:8080/>

Devices can now connect to the server.
The `/var/sota/sota.toml` file has several "server" settings that need to point
to this new server:

* `tls.server`
* `provision.server`
* `uptane.repo_server`
* `pacman.ostree_server`
* `pacman.compose_apps_proxy = "https://<HOSTNAME>:8443/app-proxy-url"`
