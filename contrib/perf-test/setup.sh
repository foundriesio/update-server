#!/bin/bash -e
# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

DATADIR=/data
NUM_DEVICES=5000
HOSTNAME=fioserver
SEED_UPDATE="${SEED_UPDATE:-0}"
UPDATE_TAG="${UPDATE_TAG:-main}"
UPDATE_NAME="${UPDATE_NAME:-perf-target-1}"

while [ $# -gt 0 ]; do
    case $1 in
        --datadir)     DATADIR=$2;     shift 2 ;;
        --num-devices) NUM_DEVICES=$2; shift 2 ;;
        --hostname)    HOSTNAME=$2;    shift 2 ;;
        --seed-update) SEED_UPDATE=1;  shift 1 ;;
        --update-tag)  UPDATE_TAG=$2;  shift 2 ;;
        --update-name) UPDATE_NAME=$2; shift 2 ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

mkdir -p "$DATADIR/auth"

# auth-init/tuf-init refuse to run against a datadir that already has an HMAC
# secret / TUF root key (by design — overwriting either is unrecoverable), so
# re-running setup against a leftover datadir fails deep inside fioserver with
# a bare "hmac secret exists at: ..." error. Catch it here instead, with a
# pointer to the fix (matching the README's "Always run `make clean` before
# re-running" warning about stale certs vs. the DB).
if [ -f "$DATADIR/auth/hmac.secret" ]; then
    echo "ERROR: $DATADIR already has an initialized fioserver datadir." >&2
    echo "Run 'make clean' (or delete $DATADIR) before re-running setup." >&2
    exit 1
fi

fioserver --datadir "$DATADIR" auth-init
fioserver --datadir "$DATADIR" tuf-init

cat > "$DATADIR/auth/auth-config.json" <<EOF
{
   "Type" : "local",
   "Config": {},
   "RateLimits": {
     "AttemptsPerSecond": 4000
   },
   "NewUserDefaultScopes" : [
      "users:read-update",
      "devices:read-update"
   ]
}
EOF

fioserver --datadir "$DATADIR" user-add \
    --username admin --password admin \
    --tokenfile "$DATADIR/auth/admin_token.txt" \
    --allowedscopes users:read-update devices:read-update devices:delete updates:read-update

gen-certs \
    --datadir "$DATADIR" \
    --num-devices "$NUM_DEVICES" \
    --hostname "$HOSTNAME" \
    $([ "$SEED_UPDATE" = "1" ] && echo --seed-update) \
    --update-tag "$UPDATE_TAG" \
    --update-name "$UPDATE_NAME"

# Start server in the background and wait until the REST API responds.
fioserver --datadir "$DATADIR" serve &
SERVER_PID=$!

echo "Waiting for server to start..."
until curl -sf http://localhost:8080/v1/devices \
        -H "Authorization: Bearer $(cat "$DATADIR/auth/admin_token.txt")"; do
    kill -0 "$SERVER_PID" 2>/dev/null || { echo "ERROR: server exited unexpectedly"; exit 1; }
    sleep 0.5
done
echo "Server ready — setup complete"

kill "$SERVER_PID"
wait "$SERVER_PID" 2>/dev/null || true
