#!/bin/bash
# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

set -euo pipefail

DATADIR="${1:-./.local-data}"

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

echo "==> Initialising auth (noauth / test mode)..."
./bin/fioserver --datadir "$DATADIR" auth-init --test

echo "==> Seeding mock devices..."
go run ./cmd/seed --datadir "$DATADIR"

echo ""
echo "UI: http://localhost:8080/devices  (noauth — no login required)"
echo ""

exec ./bin/fioserver --datadir "$DATADIR" serve
