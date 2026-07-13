# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

NUM_DEVICES ?= 5000
DATA_DIR    ?= ./perf-test-data
SEED_UPDATE ?= 0
UPDATE_TAG  ?= main
UPDATE_NAME ?= perf-target-1
NUM_ADMINS  ?= 1
SPAWN_RATE  ?= 80
RUN_TIME    ?= 5m
LOCUST_ARGS ?=

# PROFILE (scale/timing: NUM_DEVICES/SPAWN_RATE/RUN_TIME) and SCENE (task
# selection: LOCUST_ARGS) are optional named presets under profiles/ and
# scenes/ respectively — see those directories and README's "Running
# isolated scenarios" section. Included conditionally so a bare `make
# headless` with neither set is unaffected. Any value set this way can
# still be overridden on the command line: `make ... PROFILE=smoke
# NUM_DEVICES=5` uses NUM_DEVICES=5, since command-line assignments always
# take precedence over assignments from an included file.
ifneq ($(PROFILE),)
include profiles/$(PROFILE).mk
endif
ifneq ($(SCENE),)
include scenes/$(SCENE).mk
endif

COMPOSE_ENV = NUM_DEVICES=$(NUM_DEVICES) DATA_DIR=$(DATA_DIR) SEED_UPDATE=$(SEED_UPDATE) \
  UPDATE_TAG=$(UPDATE_TAG) UPDATE_NAME=$(UPDATE_NAME) NUM_ADMINS=$(NUM_ADMINS)

.PHONY: run setup headless headless-scenario \
  locust-update-check locust-update locust-admin locust-steady-state \
  clean

run: ## Web UI at http://localhost:8089
	$(COMPOSE_ENV) docker compose up --build

setup: ## Generate certs + seed auth only (no load test). Add SEED_UPDATE=1 to also seed a TUF target + rollout.
	$(COMPOSE_ENV) docker compose up --build setup

headless: ## Headless run with CSV+HTML report. Add LOCUST_ARGS="PerfUser --tags update:check" to run an isolated scenario, or use headless-scenario/locust-* below.
	$(COMPOSE_ENV) docker compose run --rm \
	  -e LOCUST_HEADLESS=true \
	  locust locust \
	    -f locustfile.py \
	    --host https://dg-sat:8443 \
	    --headless \
	    -u $(NUM_DEVICES) \
	    -r $(SPAWN_RATE) \
	    -t $(RUN_TIME) \
	    --csv /data/locust-results \
	    --html /data/locust-report.html \
	    --num-devices $(NUM_DEVICES) \
	    $(LOCUST_ARGS)

headless-scenario: headless ## Compose a named profile + scene: make headless-scenario PROFILE=smoke SCENE=update-check

# Thin sugar for the built-in scenes, so the common cases don't require
# remembering the LOCUST_ARGS/--tags incantation or passing SCENE=. These
# set LOCUST_ARGS directly rather than SCENE — SCENE only works via the
# ifneq/include above, which runs once at parse time, before any
# target-specific variable assignment like `target: SCENE = ...` ever
# takes effect, so that mechanism can't be reused here.
locust-update-check: LOCUST_ARGS = PerfUser --tags update:check
locust-update-check: headless ## Check for update only (no download); needs SEED_UPDATE=1

locust-update: LOCUST_ARGS = PerfUser --tags update
locust-update: headless ## Full check-for-update + download flow; needs SEED_UPDATE=1

locust-admin: LOCUST_ARGS = PerfAdminUser --tags admin:list-devices
locust-admin: headless ## Admin device-listing only, no device traffic

locust-steady-state: LOCUST_ARGS = PerfUser --exclude-tags update
locust-steady-state: headless ## Steady-state device traffic only, safe unseeded

clean: ## Tear down containers and delete seeded data volume (required before re-running)
	-docker compose down -v
	rm -rf $(DATA_DIR)
