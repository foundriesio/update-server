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
