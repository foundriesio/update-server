# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

# Check for update only (timestamp/snapshot/targets); no download. Requires
# SEED_UPDATE=1 (see README's "Seeding a TUF target"), or every request
# 404s.
LOCUST_ARGS := PerfUser --tags update:check
