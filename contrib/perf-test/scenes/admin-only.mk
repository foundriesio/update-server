# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

# Admin device-listing only — no mTLS device traffic at all.
LOCUST_ARGS := PerfAdminUser --tags admin:list-devices
