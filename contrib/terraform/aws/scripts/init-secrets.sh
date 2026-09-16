#!/bin/bash
# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear
#
# Generates the update server's hmac secret, PKI (root CA, TLS cert, device
# CA), and TUF keys locally, then escrows them in AWS Secrets Manager. This is
# what the instance itself used to do on first boot; running it here instead
# means the instance's IAM role never needs secretsmanager:PutSecretValue --
# it only ever reads the escrow back (see fioserver-bootstrap.sh's State B).
#
# A ready-made auth-config.json is required (--auth-config-json): this script
# does not generate a local-auth admin account for you. See docs/auth.md and
# contrib/auth-config/ for templates -- copy one and
# fill in a password/client secret to bootstrap a local admin user yourself.
#
# Run this once per deployment, BEFORE `terraform apply` -- it creates the
# Secrets Manager containers itself, and the Terraform module looks them up
# with a data source rather than creating them. Running `apply` first would
# have it error out immediately at that lookup rather than succeed and let
# the instance fail loudly at boot instead.
#
# The values passed here MUST match the corresponding Terraform variables
# exactly -- they are how this script computes the same Secrets Manager
# names and the same PKI/TUF identity Terraform expects the instance to
# restore.

set -euo pipefail

FIOSERVER=fioserver
NAME_PREFIX=fioserver
HOSTNAME=""
GATEWAY_HOSTNAME=""
FACTORY=""
TLS_EXPIRY_DAYS=3650
AUTH_CONFIG_FILE=""
REGION=""

usage() {
    cat <<'EOF'
Usage: init-secrets.sh --hostname HOST --factory NAME --auth-config-json FILE [options]

Required:
  --hostname NAME           Must match var.hostname in terraform.tfvars.
  --factory NAME            Must match var.factory in terraform.tfvars.
  --auth-config-json FILE   Path to auth-config.json. See docs/auth.md and
                             contrib/auth-config/.

Options:
  --name-prefix NAME        Must match var.name_prefix (default: fioserver).
  --gateway-hostname NAME   Must match var.gateway_hostname, if set. Defaults
                             to --hostname, matching the Terraform default.
  --tls-expiry-days N       Must match var.tls_expiry_days (default: 3650).
  --fioserver PATH          Path to the fioserver binary that matches the
                             version in the deployed AMI (default: fioserver
                             on $PATH).
  --region REGION           AWS region, if not already set via the
                             environment or an AWS CLI profile.
EOF
}

log() { echo "init-secrets: $*"; }
die() { echo "init-secrets: ERROR: $*" >&2; exit 1; }

while [ $# -gt 0 ]; do
    case "$1" in
    --name-prefix) NAME_PREFIX="$2"; shift 2 ;;
    --hostname) HOSTNAME="$2"; shift 2 ;;
    --gateway-hostname) GATEWAY_HOSTNAME="$2"; shift 2 ;;
    --factory) FACTORY="$2"; shift 2 ;;
    --tls-expiry-days) TLS_EXPIRY_DAYS="$2"; shift 2 ;;
    --auth-config-json) AUTH_CONFIG_FILE="$2"; shift 2 ;;
    --fioserver) FIOSERVER="$2"; shift 2 ;;
    --region) REGION="$2"; shift 2 ;;
    -h | --help) usage; exit 0 ;;
    *) die "unrecognized argument: $1" ;;
    esac
done

[ -n "$HOSTNAME" ] || { usage; die "--hostname is required"; }
[ -n "$FACTORY" ] || { usage; die "--factory is required"; }
[ -n "$AUTH_CONFIG_FILE" ] || { usage; die "--auth-config-json is required"; }
[ -r "$AUTH_CONFIG_FILE" ] || die "cannot read $AUTH_CONFIG_FILE"
[ -z "$GATEWAY_HOSTNAME" ] && GATEWAY_HOSTNAME="$HOSTNAME"

command -v "$FIOSERVER" >/dev/null || die "fioserver binary not found: $FIOSERVER"
command -v aws >/dev/null || die "AWS CLI not found"
[ -n "$REGION" ] && export AWS_DEFAULT_REGION="$REGION"

SECRET_PREFIX="${NAME_PREFIX}/${HOSTNAME}"

secret_id() { echo "${SECRET_PREFIX}/$1"; }

secret_exists() {
    aws secretsmanager describe-secret --secret-id "$(secret_id "$1")" >/dev/null 2>&1
}

secret_ensure() {
    local name="$1"
    secret_exists "$name" && return 0
    aws secretsmanager create-secret \
        --name "$(secret_id "$name")" \
        --description "Written by scripts/init-secrets.sh: $name" >/dev/null
    log "created secret $(secret_id "$name")"
}

secret_put() {
    local name="$1" file="$2" size
    size=$(wc -c <"$file")
    if [ "$size" -gt 65536 ]; then
        die "$name is ${size} bytes, over the 64KiB Secrets Manager limit"
    fi
    # file:// (not fileb://) -- these payloads are already base64 ASCII, and
    # passing them via a file keeps them off the process command line.
    aws secretsmanager put-secret-value \
        --secret-id "$(secret_id "$name")" \
        --secret-string "file://${file}" >/dev/null
    log "escrowed $name (${size} bytes)"
}

# Create the secret containers if they don't exist yet. This must happen
# before `terraform apply` -- the module looks these up with a data source
# rather than creating them, so `apply` fails fast if this script hasn't run.
for name in hmac-secret certs-archive tuf-archive auth-config; do
    secret_ensure "$name"
done

DATADIR="$(mktemp -d)"
trap 'rm -rf "$DATADIR"' EXIT
umask 077

log "running auth-init"
"$FIOSERVER" --datadir "$DATADIR" auth-init

install -d -m 0750 "${DATADIR}/auth"
cp "$AUTH_CONFIG_FILE" "${DATADIR}/auth/auth-config.json"
chmod 0640 "${DATADIR}/auth/auth-config.json"

log "running pki-init for ${GATEWAY_HOSTNAME} (factory ${FACTORY})"
"$FIOSERVER" --datadir "$DATADIR" pki-init \
    --dnsname "$GATEWAY_HOSTNAME" \
    --factory "$FACTORY" \
    --tlsexpirydays "$TLS_EXPIRY_DAYS"

log "running tuf-init"
"$FIOSERVER" --datadir "$DATADIR" tuf-init

log "escrowing secrets to Secrets Manager under ${SECRET_PREFIX}/"

base64 -w0 <"${DATADIR}/auth/hmac.secret" >"${DATADIR}/hmac.b64"
secret_put hmac-secret "${DATADIR}/hmac.b64"

secret_put auth-config "${DATADIR}/auth/auth-config.json"

# Archive whole directories rather than individual keys. A restore needs more
# than the roots of trust: devices authenticate against cas.pem and
# device-ca.crt, and the TUF metadata chain must match the keys. pki-init
# cannot rebuild a partial certs/ (AssertCleanPki refuses any pre-existing
# file) and there is no re-issue-leaf subcommand, so a faithful restore means
# keeping everything.
tar -czf - -C "${DATADIR}" certs | base64 -w0 >"${DATADIR}/certs.b64"
secret_put certs-archive "${DATADIR}/certs.b64"

tar -czf - -C "${DATADIR}" tuf | base64 -w0 >"${DATADIR}/tuf.b64"
secret_put tuf-archive "${DATADIR}/tuf.b64"

log "done. The instance will restore this escrow on its next boot" \
    "(or the next fioserver-bootstrap.service run)."
