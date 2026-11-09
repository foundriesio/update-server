# Advanced Topics

## Custom Listen Addresses

`serve` binds the web UI / REST API to `:8080` and the device gateway to
`:8443` by default — on all interfaces. Both are configurable:

```
  ./fioserver --datadir=./datadir serve --uiaddr=127.0.0.1:8080 --gatewayaddr=:9443
```

Binding the UI to `127.0.0.1` keeps it reachable only from the local
machine, which is worth considering with the "noauth" provider. The
gateway must remain reachable by devices; it is protected by mTLS.

> [!NOTE]
> The gateway port is baked into the server URLs of the `sota.toml` returned by
> device enrollment, so choose it before enrolling devices.

## Certificate lifetimes

The PKI commands accept validity periods in days:

* `pki-init --tlsexpirydays` — server TLS certificate validity
  (default 365).
* `pki-init --caexpirydays` — root and device CA certificate validity
  (default 7300).
* `sign-csr --expirydays` — TLS certificate validity when signing locally
  (default 365).

## Device Registration

The `POST /v1/devices` API signs a device CSR and returns the credentials plus a
complete `sota.toml` — it is the same API used by fio-device-register-compatible
tooling. To enroll a device without fio-device-register, first generate the
device's key and CSR. The CN must be the device UUID and the OU must be your
factory name:

```
  uuid=$(uuidgen)
  openssl ecparam -genkey -name prime256v1 -out pkey.pem
  openssl req -new -key pkey.pem -out device.csr -subj "/CN=${uuid}/OU=<FACTORY>"
```

Enroll it (with the "noauth" provider. Any `Authorization` value is accepted;
see the [API guide](./api.md) for real tokens):

```
  jq -n --arg uuid "$uuid" --arg name my-device --arg hwid <HARDWARE-ID> \
    --rawfile csr device.csr \
    '{uuid: $uuid, name: $name, "hardware-id": $hwid, csr: $csr}' \
  | curl -X POST http://localhost:8080/v1/devices \
      -H "Authorization: Bearer <token>" \
      -H "Content-Type: application/json" -d @- > response.json
```

The response holds three files. Install them, along with the private key,
into `/var/sota` on the device:

```
  jq -r '."root.crt"'   response.json > root.crt
  jq -r '."client.pem"' response.json > client.pem
  jq -r '."sota.toml"'  response.json > sota.toml
  # copy root.crt, client.pem, sota.toml, and pkey.pem to /var/sota/
```

The generated `sota.toml` already points `tls.server`, `provision.server`,
`uptane.repo_server`, `pacman.ostree_server`, and
`pacman.compose_apps_proxy` at this server's device gateway; no hand-editing
is needed. The gateway URL comes from the TLS certificate's DNS name plus
the gateway port, so the `--dnsname` given to `pki-init` must be resolvable
by devices. A server set up with `devserver-init --ip` uses that address
instead.
