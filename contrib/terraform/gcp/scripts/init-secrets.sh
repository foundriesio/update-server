#!/bin/bash
# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear
#
# Generates the update server's hmac secret, PKI (root CA, TLS cert, device
# CA), and TUF keys locally, then escrows them in GCP Secret Manager. This is
# what the instance itself used to do on first boot; running it here instead
# means the instance's service account never needs
# secretmanager.versions.add -- it only ever reads the escrow back (see
# fioserver-bootstrap.sh's State B).
#
# A ready-made auth-config.json is required (--auth-config-json): this script
# does not generate a local-auth admin account for you. See docs/auth.md and
# contrib/auth-config-{local,github,google}.json for templates -- copy one and
# fill in a password/client secret to bootstrap a local admin user yourself.
#
# Run this once per deployment, BEFORE `terraform apply` -- it creates the
# Secret Manager containers itself, and the Terraform module looks them up
# with a data source rather than creating them. Running `apply` first would
# have it error out immediately at that lookup rather than succeed and let
# the instance fail loudly at boot instead.
#
# The values passed here MUST match the corresponding Terraform variables
# exactly -- they are how this script computes the same Secret Manager names
# and the same PKI/TUF identity Terraform expects the instance to restore.

set -euo pipefail

FIOSERVER=fioserver
NAME_PREFIX=fioserver
HOSTNAME=""
GATEWAY_HOSTNAME=""
FACTORY=""
TLS_EXPIRY_DAYS=3650
AUTH_CONFIG_FILE=""
PROJECT=""

usage() {
    cat <<'EOF'
Usage: init-secrets.sh --hostname HOST --factory NAME --auth-config-json FILE [options]

Required:
  --hostname NAME           Must match var.hostname in terraform.tfvars.
  --factory NAME            Must match var.factory in terraform.tfvars.
  --auth-config-json FILE   Path to auth-config.json. See docs/auth.md and
                             contrib/auth-config-{local,github,google}.json.

Options:
  --name-prefix NAME        Must match var.name_prefix (default: fioserver).
  --gateway-hostname NAME   Must match var.gateway_hostname, if set. Defaults
                             to --hostname, matching the Terraform default.
  --tls-expiry-days N       Must match var.tls_expiry_days (default: 3650).
  --fioserver PATH          Path to the fioserver binary that matches the
                             version in the deployed image (default:
                             fioserver on $PATH).
  --project PROJECT         GCP project, if not already set via the
                             environment or the gcloud CLI's active config.
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
    --project) PROJECT="$2"; shift 2 ;;
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
command -v gcloud >/dev/null || die "gcloud CLI not found"
[ -n "$PROJECT" ] && export CLOUDSDK_CORE_PROJECT="$PROJECT"

# Secret Manager IDs cannot contain "/" or ".", unlike Secrets Manager's
# path-style names, so this collapses the hostname into dashes -- matching
# the naming in modules/server/main.tf's local.secret_prefix.
SECRET_PREFIX="${NAME_PREFIX}-${HOSTNAME//./-}"

secret_id() { echo "${SECRET_PREFIX}-$1"; }

secret_exists() {
    gcloud secrets describe "$(secret_id "$1")" >/dev/null 2>&1
}

secret_ensure() {
    local name="$1"
    secret_exists "$name" && return 0
    gcloud secrets create "$(secret_id "$name")" \
        --replication-policy=automatic >/dev/null
    log "created secret $(secret_id "$name")"
}

secret_put() {
    local name="$1" file="$2" size
    size=$(wc -c <"$file")
    if [ "$size" -gt 65536 ]; then
        die "$name is ${size} bytes, over the 64KiB Secret Manager limit"
    fi
    # Unlike AWS's put-secret-value, --data-file takes raw bytes directly --
    # no need to base64 the payload ourselves first. The Secret Manager REST
    # API always base64-encodes it in transit, which is what
    # fioserver-bootstrap.sh's secret_get decodes on the way back out.
    gcloud secrets versions add "$(secret_id "$name")" \
        --data-file="$file" >/dev/null
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

log "escrowing secrets to Secret Manager under ${SECRET_PREFIX}-"

secret_put hmac-secret "${DATADIR}/auth/hmac.secret"

secret_put auth-config "${DATADIR}/auth/auth-config.json"

# Archive whole directories rather than individual keys. A restore needs more
# than the roots of trust: devices authenticate against cas.pem and
# device-ca.crt, and the TUF metadata chain must match the keys. pki-init
# cannot rebuild a partial certs/ (AssertCleanPki refuses any pre-existing
# file) and there is no re-issue-leaf subcommand, so a faithful restore means
# keeping everything.
tar -czf "${DATADIR}/certs.tar.gz" -C "${DATADIR}" certs
secret_put certs-archive "${DATADIR}/certs.tar.gz"

tar -czf "${DATADIR}/tuf.tar.gz" -C "${DATADIR}" tuf
secret_put tuf-archive "${DATADIR}/tuf.tar.gz"

log "done. The instance will restore this escrow on its next boot" \
    "(or the next fioserver-bootstrap.service run)."
