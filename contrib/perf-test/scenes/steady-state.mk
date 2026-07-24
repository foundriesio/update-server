# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

# Steady-state device check-in traffic only (/device, /config, /events),
# explicitly excluding the update-check/download flow. Safe to run
# unseeded.
LOCUST_ARGS := PerfUser --exclude-tags update
