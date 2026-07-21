# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

.DEFAULT_GOAL := help

NUM_DEVICES ?= 5000
DATA_DIR    ?= ./perf-test-data
SEED_UPDATE ?= 0
UPDATE_TAG  ?= main
UPDATE_NAME ?= perf-target-1
NUM_ADMINS  ?= 1
SPAWN_RATE  ?= 80
RUN_TIME    ?= 5m
LOCUST_ARGS ?=
DRY_RUN     ?=

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

# HOST_UID/HOST_GID: containers run as the invoking host user (see
# docker-compose.yml's `user:`) so files written into the DATA_DIR bind mount
# (auth/hmac.secret, tuf/keys/*.key, etc.) come out host-user-owned instead of
# root-owned — root-owned output there used to break `make clean`'s rm -rf,
# and could make a later --build choke trying to read it as part of the
# (repo-root) build context. Named HOST_* rather than UID/GID: UID is a
# readonly shell variable in bash/dash, so `UID=... docker compose ...`
# fails outright with "UID: readonly variable".
COMPOSE_ENV = NUM_DEVICES=$(NUM_DEVICES) DATA_DIR=$(DATA_DIR) SEED_UPDATE=$(SEED_UPDATE) \
  UPDATE_TAG=$(UPDATE_TAG) UPDATE_NAME=$(UPDATE_NAME) NUM_ADMINS=$(NUM_ADMINS) \
  HOST_UID=$(shell id -u) HOST_GID=$(shell id -g)

# Docker auto-creates a bind mount's host-side source directory if it's
# missing — but the daemon does that as root, before `user:` ever applies,
# which would put us right back to a root-owned DATA_DIR. Pre-create it as
# the invoking user first so Docker just finds it already there.
define ensure_data_dir
	@mkdir -p $(DATA_DIR)
endef

RUN_CMD      = $(COMPOSE_ENV) docker compose up --build
SETUP_CMD    = $(COMPOSE_ENV) docker compose up --build setup
UP_CMD       = $(COMPOSE_ENV) docker compose up --build -d --wait fioserver
HEADLESS_CMD = $(COMPOSE_ENV) docker compose run --rm --no-deps \
	  -e LOCUST_HEADLESS=true \
	  locust locust \
	    -f locustfile.py \
	    --host https://fioserver:8443 \
	    --headless \
	    -u $(NUM_DEVICES) \
	    -r $(SPAWN_RATE) \
	    -t $(RUN_TIME) \
	    --csv /data/locust-results \
	    --html /data/locust-report.html \
	    --num-devices $(NUM_DEVICES) \
	    $(LOCUST_ARGS)

# headless/locust-* use --no-deps (see HEADLESS_CMD) so repeated scenario runs
# against an already-up stack don't re-walk the depends_on graph — Compose
# re-verifying setup's one-shot `service_completed_successfully` condition on
# every `run` recreates and reruns it even though it already exited 0, which
# then fails outright since auth-init/tuf-init refuse to run twice against an
# initialized datadir. --no-deps skips that, but also skips auto-starting
# fioserver — so check it's actually up first and point at `make up` if not,
# rather than let Locust run straight into connection-refused.
define require_fioserver_up
	@cid="$$($(COMPOSE_ENV) docker compose ps -q fioserver)"; \
	if [ -z "$$cid" ] || [ "$$(docker inspect -f '{{.State.Health.Status}}' "$$cid" 2>/dev/null)" != "healthy" ]; then \
		echo "ERROR: fioserver isn't up and healthy. Run 'make up' first." >&2; \
		exit 1; \
	fi
endef

.PHONY: help run setup up headless headless-scenario \
  locust-update-check locust-update locust-admin locust-steady-state \
  clean

help: ## Show this help
	@echo "Usage: make <target> [VAR=value ...]"
	@echo
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo
	@echo "Common variables: NUM_DEVICES, SEED_UPDATE, UPDATE_TAG, NUM_ADMINS — see README."
	@echo "DRY_RUN=1 prints the docker compose command instead of running it (works on"
	@echo "run/setup/up/headless and anything that depends on them)."
	@echo "PROFILE=<name> (scale/timing) and SCENE=<name> (task selection), used with"
	@echo "headless-scenario, e.g. \`make headless-scenario PROFILE=smoke SCENE=update-check\`:"
	@echo "  profiles: $$(ls profiles 2>/dev/null | sed 's/\.mk$$//' | tr '\n' ' ')"
	@echo "  scenes:   $$(ls scenes 2>/dev/null | sed 's/\.mk$$//' | tr '\n' ' ')"

run: ## Web UI at http://localhost:8089. Add DRY_RUN=1 to print the command instead of running it.
ifdef DRY_RUN
	@echo $(RUN_CMD)
else
	$(ensure_data_dir)
	$(RUN_CMD)
endif

setup: ## Generate certs + seed auth only (no load test). Add SEED_UPDATE=1 to also seed a TUF target + rollout.
ifdef DRY_RUN
	@echo $(SETUP_CMD)
else
	$(ensure_data_dir)
	$(SETUP_CMD)
endif

up: ## Start the persistent stack (setup once, then fioserver) so headless/locust-* can run repeatedly without redoing setup. Add DRY_RUN=1 to print the command instead of running it.
ifdef DRY_RUN
	@echo $(UP_CMD)
else
	$(ensure_data_dir)
	$(UP_CMD)
endif

headless: ## Headless run against a stack already started with `make up` (fails fast if it isn't). Add LOCUST_ARGS="PerfUser --tags update:check" for an isolated scenario, or use headless-scenario/locust-* below. Add DRY_RUN=1 to print the command instead of running it.
ifdef DRY_RUN
	@echo $(HEADLESS_CMD)
else
	$(require_fioserver_up)
	$(HEADLESS_CMD)
endif

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
