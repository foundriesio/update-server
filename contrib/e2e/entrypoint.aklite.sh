#!/bin/bash -e
#
# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear
#
# Entrypoint for the aktualizr-lite e2e client container.
#
# Adapted from aktualizr-lite/docker-e2e-test/entrypoint.sh: it stands up an
# ostree sysroot with a base commit (C0) deployed and booted, writes the sota
# conf.d snippets aktualizr-lite expects, creates the reboot-sentinel dir, and
# starts a docker daemon (this image is ubuntu-based, not docker:dind).

HARDWARE_ID=${HARDWARE_ID:-intel-corei7-64}
OS_NAME=${OS_NAME:-lmp}

# Initialize the sysroot ostree and deploy a base commit if not already present.
if [ ! -d /sysroot/ostree/repo ]; then
    echo "Initializing sysroot ostree..."
    mkdir -p /sysroot /boot /etc/ostree
    ostree admin init-fs /sysroot
    ostree admin --sysroot=/sysroot os-init "${OS_NAME}"
    ostree --repo=/sysroot/ostree/repo config set core.mode bare-user
    /usr/local/bin/make_sys_rootfs.sh initfs "${OS_NAME}" "${HARDWARE_ID}" "${OS_NAME}"
    commit=$(ostree --repo=/sysroot/ostree/repo commit initfs --branch "${OS_NAME}")
    ostree admin --sysroot=/sysroot deploy --os="${OS_NAME}" "$commit"
    rm -rf initfs
    # The device stores pulled objects as bare-user-only, matching a real device.
    ostree --repo=/sysroot/ostree/repo config set core.mode bare-user-only
fi

# Default sota config snippets (mirrors aktualizr-lite's e2e entrypoint).
mkdir -p /usr/lib/sota/conf.d /etc/sota/conf.d

sysroot_cfg=/usr/lib/sota/conf.d/z-90-sysroot.toml
if [ ! -f "$sysroot_cfg" ]; then
    printf '[pacman]\nbooted = 0\nos = "%s"\n' "${OS_NAME}" > "$sysroot_cfg"
fi

bootloader_cfg=/usr/lib/sota/conf.d/z-91-bootloader.toml
if [ ! -f "$bootloader_cfg" ]; then
    printf '[bootloader]\nreboot_command = "/usr/bin/true"\n' > "$bootloader_cfg"
fi

# aktualizr-lite creates the `need_reboot` sentinel under this dir after an
# ostree install; the test removes it to simulate a reboot.
if [ ! -d /var/run/aktualizr-session ]; then
    mkdir -p /var/run/aktualizr-session
    chmod 700 /var/run/aktualizr-session
fi

# Start a docker daemon for compose-app management (no docker:dind base here).
if [ -S /var/run/docker.sock ] || pgrep dockerd >/dev/null 2>&1; then
    :
else
    echo "Starting dockerd..."
    dockerd >/var/log/dockerd.log 2>&1 &
    for _ in $(seq 1 30); do
        docker info >/dev/null 2>&1 && break
        sleep 1
    done
fi

exec "$@"
