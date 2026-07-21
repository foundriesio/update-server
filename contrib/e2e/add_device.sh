#!/bin/bash -e
#
# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear
#
# Generate device credentials and sign the CSR with a self-signed root CA.
# Usage: add_device.sh <DATA_DIR> [HOSTNAME] [FACTORY]

DATA_DIR=$1
HOSTNAME=${2:-update-server}
FACTORY=${3:-e2e-factory}

if [ -z "$DATA_DIR" ]; then
    echo "Usage: $0 <data_dir> [hostname] [factory]"
    exit 1
fi

ls ${DATA_DIR}/certs 1>&2
mkdir -p "${DATA_DIR}/device"
openssl ecparam -genkey -name prime256v1 | openssl ec -out "${DATA_DIR}/device/pkey.pem" 2>/dev/null

DEVICE_UUID=$(cat /proc/sys/kernel/random/uuid)
cat >"${DATA_DIR}/device/device.cnf" <<EOF
[req]
prompt = no
distinguished_name = dn
req_extensions = ext

[dn]
CN = ${DEVICE_UUID}
OU = ${FACTORY}

[ext]
keyUsage=critical, digitalSignature
extendedKeyUsage=critical, clientAuth
EOF

openssl req -new -config "${DATA_DIR}/device/device.cnf" \
    -key "${DATA_DIR}/device/pkey.pem" \
    -out "${DATA_DIR}/device/device.csr"

cat >"${DATA_DIR}/device/ca.ext" <<EOF
keyUsage=keyCertSign
extendedKeyUsage=critical, clientAuth
basicConstraints=CA:TRUE
EOF

openssl x509 -req -days 3650 \
    -in "${DATA_DIR}/device/device.csr" \
    -CAcreateserial \
    -extfile "${DATA_DIR}/device/ca.ext" \
    -CAkey "${DATA_DIR}/certs/device-ca.key" \
    -CA "${DATA_DIR}/certs/device-ca.crt" \
    -out "${DATA_DIR}/device/client.pem"

cp "${DATA_DIR}/certs/root.crt" "${DATA_DIR}/device/root.crt"
rm "${DATA_DIR}/device/device.cnf" "${DATA_DIR}/device/device.csr" "${DATA_DIR}/device/ca.ext"

echo "## Device certs: ${DATA_DIR}/device/{root.crt,client.pem,pkey.pem}"
