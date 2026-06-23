#!/bin/bash
# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

set -euo pipefail

DATADIR="./.local-data"
USE_AUTH=0
AUTH_USER="${AUTH_USER:-admin}"
AUTH_PASS="${AUTH_PASS:-admin}"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --auth)
            USE_AUTH=1
            shift
            ;;
        -*)
            echo "Unknown flag: $1" >&2
            exit 1
            ;;
        *)
            DATADIR="$1"
            shift
            ;;
    esac
done

echo "==> Building fioserver..."
go build -o bin/fioserver ./cmd/server

echo "==> Preparing data directory: $DATADIR"
mkdir -p "$DATADIR/certs"

if [ ! -f "$DATADIR/certs/tls.pem" ]; then
    echo "==> Generating self-signed TLS certificate (dev only)..."
    openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes \
        -keyout "$DATADIR/certs/tls.key" \
        -out "$DATADIR/certs/tls.pem" \
        -days 3650 \
        -subj "/CN=localhost"
    cp "$DATADIR/certs/tls.pem" "$DATADIR/certs/cas.pem"
fi

if [ "$USE_AUTH" -eq 1 ]; then
    echo "==> Initialising auth (local username/password mode)..."
    ./bin/fioserver --datadir "$DATADIR" auth-init --local
else
    echo "==> Initialising auth (noauth / test mode)..."
    ./bin/fioserver --datadir "$DATADIR" auth-init --test
fi

echo "==> Seeding mock devices..."
go run ./cmd/seed --datadir "$DATADIR"

if [ "$USE_AUTH" -eq 1 ]; then
    echo "==> Creating initial user: $AUTH_USER"
    ./bin/fioserver --datadir "$DATADIR" user-add --username "$AUTH_USER" --password "$AUTH_PASS" \
        || echo "(user already exists, skipping)"

    echo ""
    echo "UI: http://localhost:8080/devices"
    echo "Login: $AUTH_USER / $AUTH_PASS"
    echo ""
else
    echo ""
    echo "UI: http://localhost:8080/devices  (noauth — no login required)"
    echo ""
fi

exec ./bin/fioserver --datadir "$DATADIR" serve
