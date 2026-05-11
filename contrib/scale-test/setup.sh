#!/bin/bash -e
# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

NUM_DEVICES=5000

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
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

if [ -z "$DATA_DIR" ] ; then
    echo "Usage: $0 --datadir <data_dir>"
    exit 1
fi

mkdir -p "$DATA_DIR/auth"

ls /data/auth
dg-sat --datadir $DATA_DIR auth-init

cat > "$DATA_DIR/auth/auth-config.json" <<EOF
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

dg-sat --datadir "$DATA_DIR" user-add \
    --username admin --password admin --tokenfile $DATA_DIR/auth/admin_token.txt \
    --allowedscopes users:read-update devices:read-update devices:delete updates:read-update

# Create all the certs we'll need
/contrib/gen-certs.sh \
    --run /usr/bin/dg-sat \
    --data-dir "$DATA_DIR" \
    --hostname dg-sat \
    --num-devices "$NUM_DEVICES"

# Start server in the background
dg-sat --datadir "$DATA_DIR" serve &
SERVER_PID=$!

# Wait for server to be ready
echo "Waiting for server to start..."
until curl -sf http://localhost:8080/v1/devices \
    -H "Authorization: Bearer $(cat $DATA_DIR/auth/admin_token.txt)"; do
    kill -0 "$SERVER_PID" 2>/dev/null || { echo "ERROR: server exited unexpectedly"; exit 1; }
    sleep 0.5
done
echo "API token verified successfully"

tar -czf /tmp/data-seeded.tgz -C $DATA_DIR ./
mv /tmp/data-seeded.tgz "$DATA_DIR/data-seeded.tgz"
