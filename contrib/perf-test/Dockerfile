# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

# Stage 1: build dg-sat and gen-certs from source
FROM public.ecr.aws/docker/library/golang:1.25 AS builder

# hack for building on QC managed laptops
RUN --mount=target=/ctx \
    if [ -d /ctx/.extra-certs/ ] ; then \
        cp /ctx/.extra-certs/* /usr/local/share/ca-certificates/ && update-ca-certificates; \
    fi

COPY go.* /dg-sat/
WORKDIR /dg-sat
RUN go mod download

COPY . /dg-sat
RUN go build -o /out/dg-sat ./cmd/server && \
    go build -o /out/gen-certs ./contrib/perf-test/gen-certs

# Stage 2: runtime image with Python + Locust
FROM public.ecr.aws/docker/library/python:3.12-slim

# hack for building on QC managed laptops
RUN --mount=target=/ctx \
    if [ -d /ctx/.extra-certs/ ] ; then \
        cp /ctx/.extra-certs/* /usr/local/share/ca-certificates/ && update-ca-certificates; \
    fi

RUN apt-get update && \
    apt-get install -y --no-install-recommends curl ca-certificates sqlite3 && \
    rm -rf /var/lib/apt/lists/*

COPY contrib/perf-test/requirements.txt /tmp/requirements.txt
RUN pip install --no-cache-dir -r /tmp/requirements.txt

COPY --from=builder /out/dg-sat   /usr/bin/dg-sat
COPY --from=builder /out/gen-certs /usr/bin/gen-certs

COPY contrib/perf-test/setup.sh   /perf/setup.sh
COPY contrib/perf-test/harness.py /perf/harness.py
COPY contrib/perf-test/locustfile.py /perf/locustfile.py
RUN chmod +x /perf/setup.sh

WORKDIR /perf
