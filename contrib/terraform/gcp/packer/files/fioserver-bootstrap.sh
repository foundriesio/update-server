#!/bin/bash
# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear
#
# First-boot bootstrap for the update server.
#
# Runs on EVERY boot (systemd oneshot) and picks one of two states. Choosing
# the wrong state is the worst thing this script can do: re-running the init
# commands on an already-provisioned server would mint a NEW TUF root key and
# orphan every enrolled device. So the ordering of these tests matters, and the
# cheapest, most local evidence (files on the data volume) is checked first.
#
#   State A  the data volume already holds a provisioned datadir  -> do nothing
#   State B  the datadir is empty and secrets are escrowed        -> restore
#

set -euo pipefail

DATADIR=/data
ENV_FILE=/etc/fioserver/env
MARKER="${DATADIR}/.bootstrap-state"
METADATA_BASE="http://metadata.google.internal/computeMetadata/v1"

log() { echo "bootstrap: $*"; }
die() { echo "bootstrap: ERROR: $*" >&2; exit 1; }

[ -r "$ENV_FILE" ] || die "missing $ENV_FILE (should be written by user_data)"
# shellcheck disable=SC1090
. "$ENV_FILE"

: "${FIOSERVER_SECRET_PREFIX:?must be set in $ENV_FILE}"

# The metadata server hands out a bearer token directly -- no SigV4-style
# request signing needed. The token is only valid for a few minutes, but
# bootstrap runs and exits well within that, so it is fetched once and reused
# for every Secret Manager call below.
GCP_PROJECT="$(curl -fsS -H 'Metadata-Flavor: Google' "${METADATA_BASE}/project/project-id")"
GCP_TOKEN="$(curl -fsS -H 'Metadata-Flavor: Google' "${METADATA_BASE}/instance/service-accounts/default/token" | jq -r '.access_token')"

secret_id() { echo "${FIOSERVER_SECRET_PREFIX}-$1"; }

# Print a secret's value exactly as Secret Manager's REST API returns it --
# base64-encoded, and left that way here so a binary payload like the HMAC
# secret survives bash command substitution intact (decoding in this
# function would risk newline-stripping or NUL-byte truncation). Callers
# decode at the point of use. Prints nothing if the secret/version is absent,
# which is how a not-yet-escrowed deployment is detected, so this must not
# trip `set -e`.
secret_get() {
    curl -fsS -H "Authorization: Bearer ${GCP_TOKEN}" \
        "https://secretmanager.googleapis.com/v1/projects/${GCP_PROJECT}/secrets/$(secret_id "$1")/versions/latest:access" \
        2>/dev/null | jq -r '.payload.data // empty' || true
}

mountpoint -q "$DATADIR" || die "$DATADIR is not a mount point"

# ---------------------------------------------------------------- State A ----
# Both the TUF root key and the HMAC secret that decrypts it are present, so
# this datadir is provisioned. Never touch it.
if [ -f "${DATADIR}/tuf/keys/root.key" ] && [ -f "${DATADIR}/auth/hmac.secret" ]; then
    log "datadir already provisioned; nothing to do"
    echo "A $(date -u +%FT%TZ)" >"$MARKER"
    exit 0
fi

# ---------------------------------------------------------------- State B ----
# Empty volume, but an escrow exists: this is a replacement instance, a
# rebuild onto a fresh disk, or the very first boot after
# scripts/init-secrets.sh has run. Restore rather than generate, so the
# factory identity and every enrolled device survive.
HMAC_B64="$(secret_get hmac-secret)"
if [ -z "$HMAC_B64" ]; then
    die "no provisioned datadir and no escrow in Secret Manager;" \
        "run contrib/terraform/gcp/scripts/init-secrets.sh before this instance's first boot"
fi

log "escrow found; restoring PKI and TUF state from Secret Manager"
install -d -m 0750 "${DATADIR}/auth" "${DATADIR}/certs" "${DATADIR}/tuf"

# Secret Manager's REST API always base64-encodes payload.data, regardless of
# what was originally stored -- unlike AWS, where only the HMAC secret needed
# manual encoding, because of our own choice at write time. Every secret below
# gets the same base64 -d treatment as a result.
umask 077
echo "$HMAC_B64" | base64 -d >"${DATADIR}/auth/hmac.secret"
chmod 0600 "${DATADIR}/auth/hmac.secret"

AUTH_CONFIG_B64="$(secret_get auth-config)"
[ -n "$AUTH_CONFIG_B64" ] || die "auth-config secret is empty; cannot restore"
echo "$AUTH_CONFIG_B64" | base64 -d >"${DATADIR}/auth/auth-config.json"
chmod 0640 "${DATADIR}/auth/auth-config.json"

# The archives are created with `tar -C $DATADIR certs` / `... tuf`, so
# their entries are already prefixed with the directory name. Extract at
# the datadir root, not into the directory itself.
for name in certs-archive tuf-archive; do
    val="$(secret_get "$name")"
    [ -n "$val" ] || die "$name secret is empty; cannot restore"
    echo "$val" | base64 -d | tar -xzf - -C "$DATADIR"
    log "restored $name"
done

# Re-assert restrictive modes: tar preserves them, but a hand-edited escrow
# might not.
find "${DATADIR}/certs" "${DATADIR}/tuf" -type f -name '*.key' -exec chmod 0600 {} +

# Deliberately NOT restored: db.sqlite. The escrow preserves the server's
# identity, not its data. If the disk was lost, recover the database from a
# snapshot; otherwise device and update records are gone even though the PKI
# is intact.
log "restore complete (db.sqlite not restored; see the README)"
echo "B $(date -u +%FT%TZ)" >"$MARKER"
