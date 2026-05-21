FROM debian:trixie
RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        qemu-system-x86 qemu-utils genisoimage ca-certificates && \
    rm -rf /var/lib/apt/lists/*
