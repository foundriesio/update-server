#!/bin/bash -e
# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

FACTORY="dg-satellite-fake"
HOSTNAME=$(hostname)
NUM_DEVICES="1"

while [ $# -gt 0 ]; do
    case $1 in
        --data-dir)
            DATA_DIR=$2
            shift 2
            ;;
        --factory)
            FACTORY=$2
            shift 2
            ;;
        --hostname)
            HOSTNAME=$2
            shift 2
            ;;
        --num-devices)
            NUM_DEVICES=$2
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

if [ -z "$DATA_DIR" ] ; then
    echo "Usage: $0 --data-dir <data_dir> [--factory <factory>]"
    exit 1
fi

DG_DIR=$(dirname $(dirname $(readlink -f $0)))

echo "Data Dir: $DATA_DIR"
echo "DG Dir: $DG_DIR"
echo "Factory: $FACTORY"
echo "Device Name: $DEVICE_NAME"
echo "Hostname: $HOSTNAME"

echo "## Generating TLS CSR"
cd ${DG_DIR}
go run github.com/foundriesio/update-server/cmd/server --datadir ${DATA_DIR} create-csr --dnsname ${HOSTNAME} --factory ${FACTORY}

cd ${DATA_DIR}/certs

echo
echo "## Creating Root CA..."
cat >ca.cnf <<EOF
[req]
prompt = no
distinguished_name = dn
x509_extensions = ext

[dn]
CN = Factory-CA

[ext]
basicConstraints=CA:TRUE
keyUsage = keyCertSign
extendedKeyUsage = critical, clientAuth, serverAuth
EOF

openssl ecparam -genkey -name prime256v1 | openssl ec -out factory_ca.key
openssl req $extra -new -x509 -days 7300 -config ca.cnf -key factory_ca.key -out factory_ca.pem
rm ca.cnf

echo
echo "## Create Devices - cheat and root crt as a 'device ca'"
cp factory_ca.pem cas.pem
cd ..
mkdir fake-devices

for x in $(seq $NUM_DEVICES) ; do
	name="device-$x"
	mkdir fake-devices/${name}
	openssl ecparam -genkey -name prime256v1 | openssl ec -out fake-devices/${name}/pkey.pem

	cat >device.cnf <<EOF
[req]
prompt = no
distinguished_name = dn
req_extensions = ext

[dn]
CN=$(uuidgen)
OU=${FACTORY}

[ext]
keyUsage=critical, digitalSignature
extendedKeyUsage=critical, clientAuth
EOF

	openssl req -new -config device.cnf -key fake-devices/${name}/pkey.pem -out device.csr
	rm device.cnf

	cat >ca.ext <<EOF
keyUsage=keyCertSign
extendedKeyUsage=critical, clientAuth
basicConstraints=CA:TRUE
EOF
	openssl x509 -req -days 3650 -in device.csr -CAcreateserial \
		-extfile ca.ext -CAkey ./certs/factory_ca.key -CA ./certs/factory_ca.pem -out fake-devices/${name}/client.pem
	rm ca.ext device.csr
	cp certs/factory_ca.pem fake-devices/${name}/root.crt
    echo $HOSTNAME > fake-devices/${name}/dghostname
done

echo
echo "## Generate TLS cert"
cd ${DG_DIR}
go run github.com/foundriesio/update-server/cmd/server --datadir ${DATA_DIR} sign-csr --cakey ${DATA_DIR}/certs/factory_ca.key --cacert ${DATA_DIR}/certs/factory_ca.pem
