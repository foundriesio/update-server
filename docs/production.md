# Production Guide

## Deploying on AWS

[contrib/terraform/aws](../contrib/terraform/aws) has Packer and Terraform
recipes that automate a single-instance AWS deployment: an AMI holding the
release binary, a persistent EBS volume for the `--datadir`, daily volume
snapshots, and the irreplaceable secrets (`auth/hmac.secret`, the TUF keys
and the device PKI) escrowed in AWS Secrets Manager so a replacement instance 
keeps the same identity.

It offers the choice to use AWS for load balancing or expose the VM directly
to internet facing traffic.

## Deploying Manually

### Enabling TLS

The "UI port", default 8080, serves unencrypted HTTP communications.
When exposing this service to the internet, you will need to secure it
with TLS. The recommended way for doing this is with a reverse proxy
handling the TLS termination.

### Caddy Reverse Proxy

The [Caddy](https://caddyserver.com) project provides an easy-to-use
reverse proxy with built-in [Let's Encrypt](https://letsencrypt.org/)
support.

A simple Caddyfile can function as a TLS terminated reverse proxy with:

```
# ./caddy/Caddyfile
dg.example.com {
    reverse_proxy update-server:8080
}
```

A Docker Compose deployment could be done with:

```
# docker-compose.yml
services:
  webserver:
    image: caddy:alpine
    volumes:
      - ./caddy/Caddyfile:/etc/caddy/Caddyfile
      - ./caddy/data:/data
      - ./caddy/config:/config
    ports:
      - 80:80
      - 443:443
    depends_on:
      - dg
  dg:
    image: <your update server container>
    command:
      - --datadir=/data
      - serve
    volumes:
      - ./data:/data
```

### Backups

The server stores all of its data under the `--datadir`. This can be
backed up as needed.

> [!IMPORTANT]
> The following files can never be lost:
>  * `<datadir>/auth/hmac.secret`
>  * `<datadir>/tuf/keys/root.key`
>  * `<datadir>/certs/root.key`
>  * `<datadir>/certs/root.crt`
>
> Store copies of both somewhere safe. If any file is lost it CANNOT be
> recovered, and you will permanently lose the ability to manage your devices.

The provided Terraform scripts keep these in AWS/GCP Secret Manager.


### HA Failover

The server has a single SQLite database file, `<datadir>/db.sqlite`.
You can use the [Litestream](https://litestream.io/) project to stream
updates to an S3 compatible bucket or NFS share.

One way to handle failover would be:

* Create an NFS mount on your primary server, `/data-nfs`.
* Move all subdirectories of `/data` to `/data-nfs`:
  * `audit`
  * `auth`
  * `certs`
  * `devices`
  * `updates`
  * `users`
* Create symlinks from those directories on `/data-nfs` to `/data`.
* Use litestream to replicate `/data/db.sqlite` to `/data-nfs`

When the failover VM comes up, it:

* mounts `/data-nfs`,
* copies `/data-nfs/db.sqlite` to `/data`.
* ensures `/data` has symlinks to all the subdirectories of `/data-nfs`.

> [!NOTE]
> It is not recommended to put a SQLite database directly on an NFS share.
