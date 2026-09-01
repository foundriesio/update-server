#!/bin/bash
# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear
#
# Bake-time provisioning: install the server, the units, and then relocate every
# mutable path so the root filesystem can be mounted read-only.
#
# Read-only root is a deliberate trade. It means OS patching is "rebuild the AMI
# and replace the instance" rather than "apt upgrade in place" -- which is safe
# here precisely because the data volume and the Secrets Manager escrow let a
# replacement instance come back with the same identity. See the README.

set -euo pipefail

FIOSERVER_VERSION="$1"
FIOSERVER_ARCH="$2"

log() { echo "== $*"; }

export DEBIAN_FRONTEND=noninteractive

log "waiting for cloud-init so apt is not contended"
cloud-init status --wait >/dev/null 2>&1 || true

log "installing base packages"
apt-get update -q
apt-get install -y -q --no-install-recommends \
    ca-certificates \
    curl \
    debian-keyring \
    debian-archive-keyring \
    apt-transport-https \
    gnupg \
    unzip \
    openssl \
    e2fsprogs

log "configuring networking statically (cloud-init's netplan renderer needs a writable /etc/netplan, which the read-only root doesn't provide)"
# Without this, ens5 comes up with no address at all: cloud-init's netplan.py
# fails to write /etc/netplan/50-cloud-init.yaml (read-only root), so neither
# netplan nor networkd ever configures the interface, and dhcpcd -- which also
# can't persist its lease file -- races networkd for the same link. A static,
# baked networkd unit plus disabling both of those removes the race entirely.
install -d -m 0755 /etc/systemd/network
cat > /etc/systemd/network/10-dhcp-en.network <<'EOF'
[Match]
Name=en* eth*

[Network]
DHCP=yes
EOF
systemctl enable systemd-networkd.service
systemctl disable dhcpcd.service || true
install -d -m 0755 /etc/cloud/cloud.cfg.d
cat > /etc/cloud/cloud.cfg.d/99-fioserver.cfg <<'EOF'
# Networking is handled entirely by the static networkd unit above; leave
# cloud-init out of it so it doesn't fail trying to render netplan config
# against a read-only /etc.
network:
  config: disabled
EOF

log "installing Caddy (enabled only in the no-load-balancer topology)"
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' \
    | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' \
    > /etc/apt/sources.list.d/caddy-stable.list
apt-get update -q
apt-get install -y -q --no-install-recommends caddy
# Both topologies share one AMI; Terraform's user_data decides which of these
# gets enabled at boot.
systemctl disable caddy || true

log "installing the AWS CLI v2"
# Debian packages awscli v1; the bootstrap needs v2 semantics for Secrets
# Manager, and hand-rolling SigV4 with curl would be far more fragile.
case "$FIOSERVER_ARCH" in
    amd64) awscli_arch=x86_64 ;;
    arm64) awscli_arch=aarch64 ;;
    *) echo "unsupported arch $FIOSERVER_ARCH" >&2; exit 1 ;;
esac
curl -fsSL "https://awscli.amazonaws.com/awscli-exe-linux-${awscli_arch}.zip" -o /tmp/awscli.zip
unzip -q /tmp/awscli.zip -d /tmp
/tmp/aws/install
rm -rf /tmp/awscli.zip /tmp/aws

log "installing the SSM Agent"
# The official Debian AMIs Packer builds on top of do not ship this agent, so
# without it SSM Session Manager has nothing on the instance to connect to.
case "$FIOSERVER_ARCH" in
    amd64) ssm_arch=debian_amd64 ;;
    arm64) ssm_arch=debian_arm64 ;;
    *) echo "unsupported arch $FIOSERVER_ARCH" >&2; exit 1 ;;
esac
curl -fsSL "https://s3.amazonaws.com/ec2-downloads-windows/SSMAgent/latest/${ssm_arch}/amazon-ssm-agent.deb" \
    -o /tmp/amazon-ssm-agent.deb
dpkg -i /tmp/amazon-ssm-agent.deb
rm -f /tmp/amazon-ssm-agent.deb
systemctl enable amazon-ssm-agent

log "installing the CloudWatch agent (disabled by default; Terraform enables it via user_data when enable_cloudwatch_logs is set)"
curl -fsSL "https://amazoncloudwatch-agent.s3.amazonaws.com/debian/${FIOSERVER_ARCH}/latest/amazon-cloudwatch-agent.deb" \
    -o /tmp/amazon-cloudwatch-agent.deb
dpkg -i /tmp/amazon-cloudwatch-agent.deb
rm -f /tmp/amazon-cloudwatch-agent.deb
systemctl disable amazon-cloudwatch-agent || true

log "installing fioserver ${FIOSERVER_VERSION} (${FIOSERVER_ARCH})"
# Releases publish bare, uncompressed binaries -- no tarball, no version in the
# filename -- so the version has to come from the release tag in the URL.
url="https://github.com/foundriesio/update-server/releases/download/${FIOSERVER_VERSION}/fioserver-linux-${FIOSERVER_ARCH}"
curl -fsSL "$url" -o /usr/local/bin/fioserver
chmod 0755 /usr/local/bin/fioserver
/usr/local/bin/fioserver --datadir=/tmp version || true

install -d -m 0755 /etc/fioserver
{
    echo "fioserver_version=${FIOSERVER_VERSION}"
    echo "fioserver_arch=${FIOSERVER_ARCH}"
    echo "fioserver_url=${url}"
    echo "fioserver_sha256=$(sha256sum /usr/local/bin/fioserver | cut -d' ' -f1)"
    echo "built_at=$(date -u +%FT%TZ)"
} > /etc/fioserver/build-info

log "installing units and scripts"
install -m 0755 /tmp/files/fioserver-bootstrap.sh /usr/local/sbin/fioserver-bootstrap
install -m 0755 /tmp/files/fioserver-volume-init /usr/local/sbin/fioserver-volume-init
install -m 0644 /tmp/files/data.mount /etc/systemd/system/data.mount
install -m 0644 /tmp/files/fioserver-volume-init.service /etc/systemd/system/
install -m 0644 /tmp/files/fioserver-bootstrap.service /etc/systemd/system/
install -m 0644 /tmp/files/fioserver.service /etc/systemd/system/
install -d -m 0755 /data
install -m 0644 /tmp/files/Caddyfile /etc/caddy/Caddyfile

systemctl enable fioserver-volume-init.service
systemctl enable data.mount
systemctl enable fioserver-bootstrap.service
systemctl enable fioserver.service

# Caddy's certificates must outlive the instance, so they live on the data
# volume. Symlinks are created at bake time; the targets appear when /data is
# mounted, and caddy.service is ordered after data.mount by a drop-in below.
rm -rf /var/lib/caddy
ln -s /data/caddy/lib /var/lib/caddy
install -d -m 0755 /etc/systemd/system/caddy.service.d
cat > /etc/systemd/system/caddy.service.d/override.conf <<'EOF'
[Unit]
Requires=data.mount fioserver-bootstrap.service
After=data.mount fioserver-bootstrap.service
[Service]
EnvironmentFile=/etc/fioserver/env
ExecStartPre=+/bin/mkdir -p /data/caddy/lib /data/caddy/config
ExecStartPre=+/bin/chown caddy:caddy /data/caddy /data/caddy/lib /data/caddy/config
Environment=XDG_CONFIG_HOME=/data/caddy/config
Environment=XDG_DATA_HOME=/data/caddy/lib
EOF

log "cleaning up"
apt-get clean
rm -rf /var/lib/apt/lists/* /tmp/files
cloud-init clean --logs || true
rm -f /etc/machine-id && touch /etc/machine-id
rm -f /var/lib/systemd/random-seed

log "provisioning complete"
