# Quick Start

## Install
Download the latest update server from:

 <https://github.com/foundriesio/update-server/releases>

Save as `fioserver`.
For Linux and Mac, make sure to `chmod +x fioserver`.

To run the server in a Docker container instead, see
[Running in a Container](./container.md).

## Configure server for development mode

Run the command:
```
  ./fioserver --datadir=./datadir devserver-init
```

This populates `datadir` with:
 * TUF metadata
 * PKI root of trust for mTLS
 * A default `admin:admin` user

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

To enroll a device without fio-device-register, see
[Device Registration](./advanced.md#device-registration).
