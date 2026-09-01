<!--
Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
SPDX-License-Identifier: BSD-3-Clause-Clear
-->

# Deploying the update server on AWS

Packer builds an AMI holding the `fioserver` release binary; Terraform deploys it
onto a single EC2 instance with a persistent EBS volume, daily snapshots, and the
server's irreplaceable secrets escrowed in AWS Secrets Manager.

Two topologies are provided:

| | `examples/with-load-balancer` | `examples/without-load-balancer` |
| --- | --- | --- |
| UI TLS | Application Load Balancer(ALB) with an ACM certificate | Caddy on the instance, Let's Encrypt |
| Device gateway | Network Load Balancer(NLB), TCP passthrough | Exposed directly on the instance |
| DNS Names | two (UI and gateway for the 2 LBs) | one |
| Extra cost | yes | no |

Both share the same modules and the same AMI. Both are dual-stack by default:
the VPC gets an Amazon-provided IPv6 CIDR, subnets get a `/64` each, and every
security-group rule that allows `0.0.0.0/0` gets an `::/0` sibling. Set
`enable_ipv6 = false` to opt out and stay IPv4-only.

In the load-balancer topology, the ALB and NLB run in `dualstack` mode and
Route53 gets an `AAAA` alias record next to each `A` record; the instance
itself stays IPv4-only, since the load balancers already terminate IPv6
client connections and forward to its private IPv4 address — giving the
instance its own public IPv6 would let the device gateway be reached
directly, bypassing the NLB.

In the Caddy topology the instance is the only public endpoint, so it gets a
public IPv6 address of its own (`ipv6_address_count = 1`) alongside its
Elastic IP. That address is *not* an EIP — Elastic IPs are IPv4-only — but it
stays stable across reboots because it's tied to the instance's network
interface rather than reassigned like an auto-assigned IPv4 would be.
Terraform creates the matching `AAAA` record automatically when
`hosted_zone_id` is set.

## Why it is shaped this way

Three properties of the server drive you will deploy.

**Port 8443 must be L4 passthrough.** The device gateway terminates TLS itself
and authenticates devices by client certificate (`ClientAuth:
VerifyClientCertIfGiven`, with the device CA in `certs/cas.pem`). Terminating
that at an L7 proxy would discard the certificate the server needs, so the NLB
forwards TCP untouched.

**Terraform never touches the TUF keys.** They are encrypted at rest with a key
derived from `auth/hmac.secret`, so both must be generated together, before the
instance boots. `scripts/init-secrets.sh` generates the hmac secret, PKI, and TUF
keys locally with the `fioserver` binary and escrows them in Secrets Manager; the
instance's IAM role only ever reads that escrow back, and no key material ever
enters Terraform state.

**The gateway hostname is effectively immutable.** `pki-init --dnsname` becomes
the first DNS SAN of the gateway certificate, and the server derives every
device-facing URL from it — each enrolled device stores those URLs in its
`sota.toml`. Changing the name later orphans existing devices.

## Prerequisites

- Terraform >= 1.5, Packer >= 1.9, AWS CLI v2, credentials with permission to
  create VPC, EC2, ELB, IAM, Secrets Manager, DLM and (optionally) Route53
  resources.
- A local `fioserver` binary matching the version in the AMI, for
  `scripts/init-secrets.sh` (release binaries are linux-amd64/linux-arm64 only;
  build from source for other platforms).

## 1. Build the AMI

```bash
cd packer
packer init .
packer build -var fioserver_version=v0.9.2 .
```

The version is required and deliberately has no default, so an AMI is always
reproducible. Releases publish bare, uncompressed binaries
(`fioserver-linux-amd64`, `fioserver-linux-arm64`), and the build records the
version and SHA256 in `/etc/fioserver/build-info`. For Graviton, add
`-var architecture=arm64` and choose a `t4g`-class `instance_type` when deploying.

The resulting AMI ID is printed at the end and written to `packer/manifest.json`.

## 2. Deploy

Generate and escrow the hmac secret, PKI, and TUF keys with
`scripts/init-secrets.sh` **before** running `terraform apply` — it creates
the Secrets Manager containers itself, and Terraform only ever reads them
back via a data source. Running `apply` first fails immediately at that
lookup, rather than succeeding and letting the instance fail loudly at boot.

```bash
cd scripts
./init-secrets.sh --hostname dg.example.com --gateway-hostname devices.example.com \
    --factory my-factory --auth-config-json /path/to/auth-config.json
```

The values passed here must match the corresponding Terraform variables
exactly — they compute the same Secrets Manager names and PKI/TUF identity
Terraform expects the instance to restore. In the load-balancer topology the
UI and the gateway need **separate hostnames**, because one DNS record
cannot alias to two different load balancers.

```bash
cd ../examples/with-load-balancer
cp terraform.tfvars.example terraform.tfvars
$EDITOR terraform.tfvars      # set ami_id, hostname, gateway_hostname, hosted_zone_id
terraform init
terraform apply
```

First boot then restores `auth-config.json`, `hmac.secret`, `certs/` and `tuf/`
from that escrow — it never generates key material itself. Watch it with:

```bash
aws ssm start-session --target "$(terraform output -raw instance_id)"
sudo journalctl -u fioserver-bootstrap -f
```

```bash
aws ssm start-session --target "$(terraform output -raw instance_id)"
sudo fioserver --datadir /data user-add --username admin --password <password>
```

To change the auth config on an already-deployed stack, populate the secret out
of band instead of re-running `init-secrets.sh` (which would mint a new hmac
secret and PKI/TUF identity):

```bash
aws secretsmanager put-secret-value \
  --secret-id "$(terraform output -raw secret_prefix)/auth-config" \
  --secret-string file://auth-config.json
```

## 3. Verify

```bash
HOST=$(terraform output -raw ui_url)
curl -sI "$HOST/favicon"            # 200
curl -sI "http://${HOST#https://}"  # 301 to HTTPS
```

With `enable_ipv6` on (the default), confirm the AAAA record resolves and is
reachable:

```bash
dig +short AAAA "${HOST#https://}"
curl -6sI "$HOST/favicon"            # 200
```

Then log in through a browser and open a device or update page. That exercises
the UI's own REST calls, which is the real test that `X-Forwarded-Proto` and the
self-call are both working. If those pages error, check `journalctl -u fioserver`
for attempts to reach `http://`.

By default that journal is only reachable through SSM on the instance itself.
Set `enable_cloudwatch_logs = true` to also ship it to CloudWatch Logs, at the
group named `/<name_prefix>/<hostname>` (retained for
`cloudwatch_log_retention_days`, 30 by default):

```bash
aws logs tail "$(terraform output -raw cloudwatch_log_group_name)" --follow
```

The CloudWatch agent is always installed in the AMI but disabled at boot; this
variable only decides whether Terraform configures and starts it. It adds
CloudWatch Logs ingestion/storage cost on top of the resources below.

To verify device mTLS, generate a device certificate against the deployed PKI and
use it (on the instance, where `/data` is the datadir).

## What is stored where

`/data` is the server's `--datadir` and the only writable location on the
instance:

```
/data/db.sqlite          devices, users, updates, sessions
/data/auth/              hmac.secret, auth-config.json
/data/certs/             tls.{key,pem}, cas.pem, root.{key,crt}, device-ca.{key,crt}
/data/tuf/               role keys and root metadata
/data/caddy/             Let's Encrypt certificates (no-load-balancer topology)
```

Secrets Manager holds, under `<name_prefix>/<hostname>/`:

| Secret | Written by | Contents |
| --- | --- | --- |
| `auth-config` | `scripts/init-secrets.sh` | `auth-config.json` |
| `hmac-secret` | `scripts/init-secrets.sh` | `auth/hmac.secret`, base64 |
| `certs-archive` | `scripts/init-secrets.sh` | gzipped tar of `certs/` |
| `tuf-archive` | `scripts/init-secrets.sh` | gzipped tar of `tuf/` |

Whole directories are archived rather than individual keys because a restore
needs more than the roots of trust: devices authenticate against `cas.pem` and
`device-ca.crt`, and the TUF metadata chain must match the keys. `pki-init`
cannot rebuild a partial `certs/` — it refuses to run if any file there exists.

> [!IMPORTANT]
> The escrow preserves the server's **identity**, not its **data**. `db.sqlite`
> is not in it. If the volume is lost, the PKI comes back from Secrets Manager
> but device and update records come back only from a snapshot.

## Runbooks

### Restoring onto a fresh volume

This is automatic. Boot a new instance with an empty data volume and the same
`FIOSERVER_SECRET_PREFIX` (i.e. the escrow already populated by
`scripts/init-secrets.sh`); the bootstrap detects it and restores
`hmac.secret`, `auth-config.json`, `certs/` and `tuf/` byte-for-byte, so
already-enrolled devices keep working without re-enrolment.

To also recover the database, restore the most recent snapshot into a volume,
attach it, and copy `db.sqlite`, `db.sqlite-wal` and `db.sqlite-shm` into `/data`
while `fioserver` is stopped.

Check which path a boot took:

```bash
cat /data/.bootstrap-state   # "A" reboot, "B" restored from escrow
```

## Backups

A Data Lifecycle Manager policy snapshots the data volume every 24 hours and
deletes snapshots older than `snapshot_retention_days` (60 by default). The
module creates its own DLM IAM role: `AWSDataLifecycleManagerDefaultRole` is
created implicitly by the AWS console but never by the API, so relying on it
makes `apply` fail on a fresh account.

> [!NOTE]
> These snapshots are crash-consistent, not application-consistent. SQLite in WAL
> mode recovers from them in practice, but a pre-snapshot `sqlite3 .backup` or
> [Litestream](https://litestream.io/) replication would be strictly better. See
> [docs/production.md](../../../docs/production.md).

## Limitations

- **One instance, one AZ.** The load balancers span two subnets because an ALB
  requires it, but there is a single instance and a single volume. The server
  keeps one SQLite database with `MaxOpenConns(1)`, so horizontal scaling is not
  available.
- **`terraform destroy` deletes the data volume.** Snapshots and the secret
  escrow are the recovery path. There is deliberately no `prevent_destroy`, since
  that would make `destroy` fail and leave the volume stranded. The secret
  escrow itself is untouched by `destroy` — Terraform only ever reads it — so
  re-applying against the same hostname finds it intact.
