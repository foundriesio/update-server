# Migrating a FoundriesFactory Fleet

This guide is for users with an existing FoundriesFactory® Factory with
provisioned devices who want this server to take over device management and
updates. Follow the [Quick Start](./quick-start.md), replacing two of its steps
with the factory-aware versions below so that already-provisioned devices keep
trusting the server:

- **Configure Mutual TLS** → [Sign with your factory PKI](#sign-with-your-factory-pki)
- **Initialize TUF** → [Importing the fleet's TUF root](#importing-the-fleets-tuf-root)

Then [repoint existing devices](#repoint-existing-devices) at the new
server.

## Sign with Your Factory PKI

Devices authenticate using mTLS and your FoundriesFactory PKI. You will
need access to your [Factory CA](https://docs.foundries.io/latest/reference-manual/security/device-gateway.html)
in order to create a TLS certificate for device-facing APIs.

Use this flow in place of the Quick Start's `pki-init` step.

### Create Certificate Signing Requests for TLS

Devices need to trust the TLS connection they make to this server. In
order to do this, you must create a CSR to be signed with the Factory
root key:

```
  ./fioserver --datadir=./datadir create-csr --dnsname <HOSTNAME> --factory <FACTORY>
```

### Sign the Request

There are two ways to sign the CSR, depending on where your Factory root key
lives.

On a **separate PKI machine**, Copy `datadir/certs/tls.csr` to the computer with
your factory PKI. This file does not contain sensitive information, so it is
safe to share as needed. From the Factory PKI directory run:

```
  fioctl keys ca sign --pki-dir <path to your factory pki> <path to tls.csr>
```

This command will print the contents of the certificate. Go back to the update
server system and create the file `datadir/certs/tls.crt` with this content.

**Locally with `sign-csr`.** If the Factory root key and certificate are
available as files on this machine, the server can sign its own CSR:

```
  ./fioserver --datadir=./datadir sign-csr --cakey <root.key> --cacert <root.crt>
```

This writes `datadir/certs/tls.crt` directly.

### Grant Access to Devices

This service needs to know what devices can connect to it. To allow all valid
Factory devices:
```
 fioctl keys ca show --just-device-cas > datadir/certs/cas.pem
```

Continue with the Quick Start's
[Configure User Authentication](./quick-start.md#configure-user-authentication)
step.

## Importing the Fleet's TUF Root

If you already have a fleet of devices provisioned against a Foundries.io
Factory, initializing a brand new TUF root would leave those devices unable to
validate metadata from this server. Instead, you can import the Factory's
existing root of trust so that already-provisioned devices continue to trust
updates.

The import reads every version of the Factory's `root.json` from a tarball you
provide, storing them so devices can walk the trust chain. It then generates a
new root (version N+1) with fresh online keys for the `root`, `targets`,
`snapshot`, and `timestamp` roles. The new root is signed by both the Factory's
offline root key (proving continuity of trust) and the new root key.

You will need:

- The Factory's offline keys tarball (typically `offline-creds.tgz`),
  which contains the offline root key used to sign the rotation.
- A gzipped tarball containing all of the Factory's `root.json` files (e.g.
  `1.root.json`, `2.root.json`, ...). See `fioctl keys tuf download-roots`.
```
  ./fioserver --datadir=./datadir tuf-init \
    --import-keys ./offline-creds.tgz \
    --import-roots ./roots.tgz
```

Options:

- `--import-keys` — path to the fioctl offline keys tarball. Providing this
  (or `--import-roots`) enables import mode.
- `--import-roots` — path to a gzipped tarball containing all of the Factory's
  `root.json` files. See `fioctl keys tuf download-roots`.

> [!NOTE]
> Like a plain `tuf-init`, importing requires `auth-init` to have been run
> first, and the backup warning in the Quick Start's [Initialize
> TUF](./quick-start.md#initialize-tuf) step applies to the newly generated root
> key.

## Repoint Existing Devices

Already-provisioned devices keep their Factory-issued client certificates; the
[Grant Access to Devices](#grant-access-to-devices) step is what lets them
authenticate here. Each device's `/var/sota/sota.toml` must be updated to point
its server settings at this server's device gateway:

```
[tls]
server = "https://<HOSTNAME>:8443"

[provision]
server = "https://<HOSTNAME>:8443"

[uptane]
repo_server = "https://<HOSTNAME>:8443/repo"

[pacman]
ostree_server = "https://<HOSTNAME>:8443/ostree"
compose_apps_proxy = "https://<HOSTNAME>:8443/app-proxy-url"
```

Because the server's TLS certificate is signed by the Factory root, devices
continue to trust the connection with the `root.crt` they already have.

New devices can be enrolled as described in the Quick Start's
[Enroll a Device](./quick-start.md#enroll-a-device) section, or provisioned
with images built per [How to build an Update](./build-an-update.md).
