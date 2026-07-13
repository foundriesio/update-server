# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

# Today's implicit default, made explicit and reusable by name. Spawn rate
# stays at 80/s regardless of scale — see README's "Notes" section on why
# raising it isn't recommended without checking error rates.
NUM_DEVICES := 5000
SPAWN_RATE  := 80
RUN_TIME    := 5m
