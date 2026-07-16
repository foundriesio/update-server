#!/bin/bash -e
# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear
#
# Generate PKI for e2e tests: root CA, server TLS cert, device cert.
# Usage: gen_pki.sh <DATA_DIR> <FIOSERVER_BIN> [HOSTNAME] [FACTORY]

DATA_DIR=$1
FIOSERVER_BIN=$2
HOSTNAME=${3:-update-server}
FACTORY=${4:-e2e-factory}

if [ -z "$DATA_DIR" ] || [ -z "$FIOSERVER_BIN" ]; then
    echo "Usage: $0 <data_dir> <fioserver_bin> [hostname] [factory]"
    exit 1
fi

mkdir -p "${DATA_DIR}/certs"

echo "## Generating TLS CSR via fioserver"
"$FIOSERVER_BIN" --datadir "$DATA_DIR" create-csr --dnsname "$HOSTNAME" --factory "$FACTORY"

echo "## Creating Root CA (OU=e2e-factory)"
cd "${DATA_DIR}/certs"
cat >ca.cnf <<EOF
[req]
prompt = no
distinguished_name = dn
x509_extensions = ext

[dn]
CN = Factory-CA
OU = ${FACTORY}

[ext]
basicConstraints=CA:TRUE
keyUsage = keyCertSign
extendedKeyUsage = critical, clientAuth, serverAuth
EOF

openssl ecparam -genkey -name prime256v1 | openssl ec -out factory_ca.key 2>/dev/null
openssl req -new -x509 -days 7300 -config ca.cnf -key factory_ca.key -out factory_ca.pem
rm ca.cnf

# cas.pem is the device CA list used by the server for mTLS
cp factory_ca.pem cas.pem

echo "## Signing TLS CSR"
cd "$DATA_DIR"
"$FIOSERVER_BIN" --datadir "$DATA_DIR" sign-csr \
    --cakey "${DATA_DIR}/certs/factory_ca.key" \
    --cacert "${DATA_DIR}/certs/factory_ca.pem"

echo "## Creating device credentials"
mkdir -p "${DATA_DIR}/device"
openssl ecparam -genkey -name prime256v1 | openssl ec -out "${DATA_DIR}/device/pkey.pem" 2>/dev/null

DEVICE_UUID=$(python3 -c "import uuid; print(uuid.uuid4())" 2>/dev/null || cat /proc/sys/kernel/random/uuid)
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
    -CAkey "${DATA_DIR}/certs/factory_ca.key" \
    -CA "${DATA_DIR}/certs/factory_ca.pem" \
    -out "${DATA_DIR}/device/client.pem"

cp "${DATA_DIR}/certs/factory_ca.pem" "${DATA_DIR}/device/root.crt"
rm "${DATA_DIR}/device/device.cnf" "${DATA_DIR}/device/device.csr" "${DATA_DIR}/device/ca.ext"

echo "## PKI generation complete"
echo "  Server TLS:   ${DATA_DIR}/certs/tls.pem"
echo "  Device certs: ${DATA_DIR}/device/{root.crt,client.pem,pkey.pem}"
