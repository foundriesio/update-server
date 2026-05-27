#!/bin/bash -e
# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

NUM_DEVICES=5000
OSTREE_TAG="main"
OSTREE_UPDATE_NAME="e2e"
OSTREE_ROLLOUT_NAME="ostree-scale"
API_BASE_URL="http://localhost:8080"
GATEWAY_HOST="dg-sat"
GATEWAY_ADDR="127.0.0.1"

while [ $# -gt 0 ]; do
    case $1 in
        --datadir)
            DATA_DIR=$2
            shift 2
            ;;
        --num-devices)
            NUM_DEVICES=$2
            shift 2
            ;;
        --tag)
            OSTREE_TAG=$2
            shift 2
            ;;
        --update-name)
            OSTREE_UPDATE_NAME=$2
            shift 2
            ;;
        --rollout-name)
            OSTREE_ROLLOUT_NAME=$2
            shift 2
            ;;
        --api-base-url)
            API_BASE_URL=$2
            shift 2
            ;;
        --gateway-host)
            GATEWAY_HOST=$2
            shift 2
            ;;
        --gateway-addr)
            GATEWAY_ADDR=$2
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

if [ -z "$DATA_DIR" ] ; then
    echo "Usage: $0 --datadir <data_dir> [--num-devices N]"
    exit 1
fi

TOKEN_FILE="$DATA_DIR/auth/admin_token.txt"
if [ ! -f "$TOKEN_FILE" ]; then
    echo "ERROR: Missing token file: $TOKEN_FILE"
    exit 1
fi

echo "Waiting for API server to be ready at ${API_BASE_URL}..."
until curl -sf "${API_BASE_URL}/v1/devices" \
    -H "Authorization: Bearer $(cat "$TOKEN_FILE")" > /dev/null; do
    sleep 0.5
done

echo "Preparing OSTree update archive"
OSTREE_UPDATE_DIR=$(mktemp -d)
UUIDS_FILE=$(mktemp)
ROLLOUT_PAYLOAD_FILE=$(mktemp)
mkdir -p "$OSTREE_UPDATE_DIR/tuf"
cp -a /usr/share/ostree_repo "$OSTREE_UPDATE_DIR/ostree_repo"

cat > "$OSTREE_UPDATE_DIR/tuf/root.json" <<EOF
{"signed":{}}
EOF

cat > "$OSTREE_UPDATE_DIR/tuf/targets.json" <<EOF
{"signed":{"targets":{"e2e":{"custom":{"tags":["${OSTREE_TAG}"]}}}}}
EOF

tar -C "$OSTREE_UPDATE_DIR" -czf /tmp/ostree-update.tgz tuf ostree_repo

echo "Uploading OSTree update /v1/updates/ci/${OSTREE_TAG}/${OSTREE_UPDATE_NAME}"
curl -sf \
    -H "Authorization: Bearer $(cat "$TOKEN_FILE")" \
    -H "Content-Type: application/gzip" \
    -X POST \
    --data-binary @/tmp/ostree-update.tgz \
    "${API_BASE_URL}/v1/updates/ci/${OSTREE_TAG}/${OSTREE_UPDATE_NAME}" > /dev/null

FIRST_UUID=""
for i in $(seq 1 "$NUM_DEVICES"); do
    device_dir="$DATA_DIR/fake-devices/device-$i"
    device_cert="$device_dir/client.pem"
    device_key="$device_dir/pkey.pem"
    device_ca="$device_dir/root.crt"

    device_subject=$(openssl x509 -in "$device_cert" -noout -subject -nameopt RFC2253)
    device_uuid=$(printf '%s\n' "$device_subject" | awk -F'CN=' '{print $2}' | cut -d',' -f1)
    if [ -z "$device_uuid" ]; then
        echo "ERROR: Unable to extract UUID from $device_cert (subject: $device_subject)"
        exit 1
    fi

    if [ -z "$FIRST_UUID" ]; then
        FIRST_UUID="$device_uuid"
    fi
    echo "$device_uuid" >> "$UUIDS_FILE"
done

if [ -z "$FIRST_UUID" ]; then
    echo "ERROR: No device UUIDs discovered"
    exit 1
fi

echo "$FIRST_UUID" > "$DATA_DIR/auth/ostree_device_uuid.txt"
awk 'BEGIN{printf "{\"uuids\":["} {if (NR>1) printf ","; printf "\"%s\"", $0} END{printf "]}"}' "$UUIDS_FILE" > "$ROLLOUT_PAYLOAD_FILE"

echo "Creating rollout /v1/updates/ci/${OSTREE_TAG}/${OSTREE_UPDATE_NAME}/rollouts/${OSTREE_ROLLOUT_NAME}"
curl -sf \
    -H "Authorization: Bearer $(cat "$TOKEN_FILE")" \
    -H "Content-type: application/json" \
    -X PUT \
    --data-binary @"$ROLLOUT_PAYLOAD_FILE" \
    "${API_BASE_URL}/v1/updates/ci/${OSTREE_TAG}/${OSTREE_UPDATE_NAME}/rollouts/${OSTREE_ROLLOUT_NAME}" > /dev/null

echo "Waiting for rollout assignment to be committed for device ${FIRST_UUID}"
until curl -sf \
    -H "Authorization: Bearer $(cat "$TOKEN_FILE")" \
    "${API_BASE_URL}/v1/devices/${FIRST_UUID}" | grep -q "\"update-name\":\"${OSTREE_UPDATE_NAME}\""; do
    sleep 0.5
done

echo "OSTree rollout assignment verified"
rm -rf "$OSTREE_UPDATE_DIR" "$UUIDS_FILE" "$ROLLOUT_PAYLOAD_FILE" /tmp/ostree-update.tgz
