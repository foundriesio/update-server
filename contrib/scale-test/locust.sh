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
        --locustfile)
            LOCUSTFILE=$2
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

if [ -z "$DATA_DIR" ] ; then
    echo "Usage: $0 --datadir <data_dir> --locustfile <locustfile>"
    exit 1
fi

cp /contrib/scale-test/$LOCUSTFILE /tmp/locustfile.py 

# Wait for server to be ready
echo "Waiting for server to start..."
until curl http://dg-sat:8080/v1/devices \
    -H "Authorization: Bearer $(cat $DATA_DIR/auth/admin_token.txt)"; do
    sleep 0.5
done
echo "Server is ready"

cd /tmp
results="$DATA_DIR/locust-results.$( date +%Y%m%d-%H%M%S )"
mkdir -p "$results"
locust -f locustfile.py \
    --host https://dg-sat:8443 \
    --autostart \
    --users "$NUM_DEVICES" \
    --spawn-rate 80 \
    --csv "$results/locust-results" \
    --html "$results/locust-report.html"