# Quick Start

## Install
Download the latest update server from:

 <https://github.com/foundriesio/update-server/releases>

Save as `fioserver`.
For Linux and Mac, make sure to `chmod +x fioserver`.

To run the server in a Docker container instead, see
[Running in a Container](./container.md).

## Configure Mutual TLS

Devices authenticate with the server using mutual TLS. `pki-init` creates
everything needed in one step:

```
  ./fioserver --datadir=./datadir pki-init --dnsname <HOSTNAME> --factory <FACTORY>
```

This generates, under `datadir/certs`:

* a self-signed root CA (`root.key`/`root.crt`) with `OU=<FACTORY>`,
* the server TLS keypair signed by the root CA (`tls.key`/`tls.crt`),
* an intermediate device-signing CA (`device-ca.key`/`device-ca.crt`), and
* `cas.pem`, containing the device CA so devices can authenticate.

Import `root.crt` into your devices' trust store so they trust the server's
TLS certificate.

> [!NOTE]
> If migrating an existing FoundriesFactory fleet, sign the server's TLS
> certificate with your Factory PKI instead. See [Sign with your Factory
> PKI](./migration.md#sign-with-your-factory-pki).

## Configure User Authentication

The update server includes a few [authentication providers](./auth.md)
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

> [!NOTE]
> `tuf-init` requires `auth-init` to have been run first — the TUF role keys are
> encrypted at rest with the HMAC secret it creates.

> [!IMPORTANT]
> A successful `tuf-init` generates a new root key at
> `<datadir>/tuf/keys/root.key`. This key is encrypted at rest using the HMAC
> secret at `<datadir>/auth/hmac.secret`, so you must back up BOTH files — the
> `root.key` is useless without the `hmac.secret` needed to decrypt it. Store
> copies of both somewhere safe immediately. If either file is lost it CANNOT be
> recovered, and you will permanently lose the ability to rotate or manage your
> TUF root of trust.

> [!NOTE]
> If you already have a fleet provisioned against a Foundries.io Factory, import
> its existing TUF root instead of creating a new one. See [Importing the
> fleet's TUF root](./migration.md#importing-the-fleets-tuf-root).

## Run the Server

`./fioserver --datadir=./datadir serve`

You can browse the UI at <http://localhost:8080/>

Devices can now be enrolled.

## Enroll a Device

Devices authenticate with the server using Mutual TLS. Enrollment uses
[fio-device-register](https://github.com/foundriesio/lmp-device-register), which
is part of a Yocto Project build with the
[meta-foundries](https://github.com/foundriesio/meta-foundries) components
enabled (see [How to build an Update](./build-an-update.md)). Alternatively,
[fioup](https://github.com/foundriesio/fioup/releases) can enroll a device. To
enroll with fio-device-register, run this on the device:

```
  DEVICE_API=http://<HOSTNAME>:8080/v1/devices \
  OAUTH_BASE=http://<HOSTNAME>:8080/oauth2 \
  fio-device-register \
    --factory <FACTORY> \
    --name <device-name> \
    --tags <tag>
```

`--factory` must match the Factory name given to `pki-init`.

> [!NOTE]
> The "noauth" provider does not validate tokens or their scopes. See the [API
> guide](./api.md) for real tokens.

To enroll a device without fio-device-register, see
[Device Registration](./advanced.md#device-registration).
