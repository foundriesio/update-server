# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

# Check for update only (timestamp/snapshot/targets); no download. Needs
# a seeded rollout — on by default (SEED_UPDATE=1, see README's "TUF target
# seeding"), or every request 404s.
LOCUST_ARGS := PerfUser --tags update:check
