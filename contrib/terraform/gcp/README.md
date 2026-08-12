<!--
Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
SPDX-License-Identifier: BSD-3-Clause-Clear
-->

# Deploying the update server on GCP

Packer builds a Compute Engine image holding the `fioserver` release binary;
Terraform deploys it onto a single instance with a persistent disk, daily
snapshots, and the server's irreplaceable secrets escrowed in Secret Manager.

The `examples/with-load-balancer` example uses a global external HTTPS load
balancer with a Google-managed certificate for the UI and a regional external
passthrough Network Load Balancer for the device gateway. The two load balancers
use separate DNS names.

## Why it is shaped this way

Three properties of the server drive how you will deploy it.

**Port 8443 must be L4 passthrough.** The device gateway terminates TLS
itself and authenticates devices by client certificate (`ClientAuth:
VerifyClientCertIfGiven`, with the device CA in `certs/cas.pem`). Terminating
that at an L7 proxy would discard the certificate the server needs, so the
regional Network LB forwards TCP untouched.

**Terraform never touches the TUF keys.** They are encrypted at rest with a
key derived from `auth/hmac.secret`, so both must be generated together,
before the instance boots. `scripts/init-secrets.sh` generates the hmac
secret, PKI, and TUF keys locally with the `fioserver` binary and escrows
them in Secret Manager; the instance's service account only ever reads that
escrow back, and no key material ever enters Terraform state.

**The gateway hostname is effectively immutable.** `pki-init --dnsname`
becomes the first DNS SAN of the gateway certificate, and the server derives
every device-facing URL from it — each enrolled device stores those URLs in
its `sota.toml`. Changing the name later orphans existing devices.

## Prerequisites

- Terraform >= 1.5, Packer >= 1.9 with the `googlecompute` plugin, the
  `gcloud` CLI, and a project with the Compute Engine, Secret Manager, Cloud
  DNS (if used), and IAP APIs enabled.
- Credentials with permission to create VPC, Compute Engine, load balancer,
  IAM, Secret Manager, and (optionally) Cloud DNS resources.
- A local `fioserver` binary matching the version in the image, for
  `scripts/init-secrets.sh` (release binaries are linux-amd64/linux-arm64
  only; build from source for other platforms).

## 1. Build the image

```bash
cd packer
packer init .
packer build -var project_id=my-gcp-project -var fioserver_version=v0.9.2 .
```

The version is required and deliberately has no default, so an image is
always reproducible. Releases publish bare, uncompressed binaries
(`fioserver-linux-amd64`, `fioserver-linux-arm64`), and the build records the
version and SHA256 in `/etc/fioserver/build-info`. For Arm, add
`-var architecture=arm64`.

The resulting image name is printed at the end and written to
`packer/manifest.json`.

## 2. Deploy

Generate and escrow the hmac secret, PKI, and TUF keys with
`scripts/init-secrets.sh` **before** running `terraform apply` — it creates
the Secret Manager containers itself, and Terraform only ever reads them
back via a data source. Running `apply` first fails immediately at that
lookup, rather than succeeding and letting the instance fail loudly at boot.

```bash
cd scripts
./init-secrets.sh --hostname dg.example.com --gateway-hostname devices.example.com \
    --auth-config-json /path/to/auth-config.json \
    --project my-gcp-project
```

The values passed here must match the corresponding Terraform variables
exactly — they compute the same Secret Manager names and PKI/TUF identity
Terraform expects the instance to restore. The UI and the gateway need **separate
hostnames**, because one DNS record cannot point at both reserved IPs.

```bash
cd ../examples/with-load-balancer
cp terraform.tfvars.example terraform.tfvars
$EDITOR terraform.tfvars      # set project_id, image, hostname, gateway_hostname, managed_zone_name
terraform init
terraform apply
```

First boot then restores `auth-config.json`, `hmac.secret`, `certs/` and
`tuf/` from that escrow — it never generates key material itself. Watch it
with:

```bash
gcloud compute ssh "$(terraform output -raw instance_name)" --tunnel-through-iap --zone us-central1-a
sudo journalctl -u fioserver-bootstrap -f
```

```bash
gcloud compute ssh "$(terraform output -raw instance_name)" --tunnel-through-iap --zone us-central1-a
sudo fioserver --datadir /data user-add --username admin --password <password>
```

To change the auth config on an already-deployed stack, populate the secret
out of band instead of re-running `init-secrets.sh` (which would mint a new
hmac secret and PKI/TUF identity):

```bash
gcloud secrets versions add "$(terraform output -raw secret_prefix)-auth-config" \
    --data-file=auth-config.json
```

## 3. Verify

```bash
HOST=$(terraform output -raw ui_url)
curl -sI "$HOST/favicon"            # 200
curl -sI "http://${HOST#https://}"  # 301 to HTTPS
```

The UI's Google-managed certificate stays in `PROVISIONING` until `var.hostname`'s
A record resolves to the reserved IP (`terraform output ui_ip`), which can take a
few minutes after DNS propagates. Check its status with (substitute your
`var.name_prefix`, `fioserver` by default):

```bash
gcloud compute ssl-certificates describe fioserver-ui --global
```

Then log in through a browser and open a device or update page. That
exercises the UI's own REST calls, which is the real test that
`X-Forwarded-Proto` and the self-call are both working. If those pages
error, check `journalctl -u fioserver` for attempts to reach `http://`.

By default that journal is only reachable through IAP on the instance
itself. Set `enable_cloud_logging = true` to also ship it to Cloud Logging
via the Ops Agent:

```bash
gcloud logging read 'logName:"fioserver_journald"' --project my-gcp-project --limit 50
```

The Ops Agent is always installed in the image but disabled at boot; this
variable only decides whether Terraform's `user_data` starts it. It adds
Cloud Logging ingestion/storage cost on top of the resources below, and —
unlike the AWS module's CloudWatch agent config, which can be scoped to one
unit — ships the whole systemd journal, not just `fioserver.service`'s
output.

To verify device mTLS, generate a device certificate against the deployed
PKI and use it (on the instance, where `/data` is the datadir).

## What is stored where

`/data` is the server's `--datadir` and the only writable location on the
instance:

```
/data/db.sqlite          devices, users, updates, sessions
/data/auth/              hmac.secret, auth-config.json
/data/certs/             tls.{key,pem}, cas.pem, root.{key,crt}, device-ca.{key,crt}
/data/tuf/               role keys and root metadata
```

Secret Manager holds, under `<name_prefix>-<hostname-with-dashes>-`:

| Secret | Written by | Contents |
| --- | --- | --- |
| `auth-config` | `scripts/init-secrets.sh` | `auth-config.json` |
| `hmac-secret` | `scripts/init-secrets.sh` | `auth/hmac.secret` |
| `certs-archive` | `scripts/init-secrets.sh` | gzipped tar of `certs/` |
| `tuf-archive` | `scripts/init-secrets.sh` | gzipped tar of `tuf/` |

Whole directories are archived rather than individual keys because a restore
needs more than the roots of trust: devices authenticate against `cas.pem`
and `device-ca.crt`, and the TUF metadata chain must match the keys.
`pki-init` cannot rebuild a partial `certs/` — it refuses to run if any file
there exists.

> [!IMPORTANT]
> The escrow preserves the server's **identity**, not its **data**.
> `db.sqlite` is not in it. If the disk is lost, the PKI comes back from
> Secret Manager but device and update records come back only from a
> snapshot.

## Runbooks

### Updating the VM base image

The fioserver VM instance requires you to explictly call "replace" to prevent
accidental changes/reboots of the service. In order to update its base OS
image you must run:

`terraform plan|apply -replace='module.server.google_compute_instance.server'`

This process will also re-create the load-balancers and can take a few
minutes to apply.

### Restoring onto a fresh disk

This is automatic. Boot a new instance with an empty data disk and the same
`FIOSERVER_SECRET_PREFIX` (i.e. the escrow already populated by
`scripts/init-secrets.sh`); the bootstrap detects it and restores
`hmac.secret`, `auth-config.json`, `certs/` and `tuf/` byte-for-byte, so
already-enrolled devices keep working without re-enrolment.

To also recover the database, restore the most recent snapshot into a disk,
attach it, and copy `db.sqlite`, `db.sqlite-wal` and `db.sqlite-shm` into
`/data` while `fioserver` is stopped.

Check which path a boot took:

```bash
cat /data/.bootstrap-state   # "A" reboot, "B" restored from escrow
```

## Backups

A `google_compute_resource_policy` snapshot schedule snapshots the data disk
every 24 hours and deletes snapshots older than `snapshot_retention_days`
(60 by default).

> [!NOTE]
> These snapshots are crash-consistent, not application-consistent. SQLite in
> WAL mode recovers from them in practice, but a pre-snapshot
> `sqlite3 .backup` or [Litestream](https://litestream.io/) replication would
> be strictly better. See [docs/production.md](../../../docs/production.md).
