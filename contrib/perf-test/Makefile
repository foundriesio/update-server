# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

NUM_DEVICES ?= 5000
DATA_DIR    ?= ./perf-test-data
SEED_UPDATE ?= 0
UPDATE_TAG  ?= main
UPDATE_NAME ?= perf-target-1
NUM_ADMINS  ?= 1
UPDATE_FLOW_WEIGHT ?= 0

COMPOSE_ENV = NUM_DEVICES=$(NUM_DEVICES) DATA_DIR=$(DATA_DIR) SEED_UPDATE=$(SEED_UPDATE) \
  UPDATE_TAG=$(UPDATE_TAG) UPDATE_NAME=$(UPDATE_NAME) NUM_ADMINS=$(NUM_ADMINS) \
  UPDATE_FLOW_WEIGHT=$(UPDATE_FLOW_WEIGHT)

.PHONY: run setup headless clean

setup: ## Generate certs + seed auth only (no load test). Add SEED_UPDATE=1 to also seed a TUF target + rollout.
	$(COMPOSE_ENV) docker compose up --build setup

run: ## Web UI at http://localhost:8089 (runs setup first via compose depends_on)
	$(COMPOSE_ENV) docker compose up --build

headless: ## Headless run with CSV+HTML report (spawn-rate capped at 80/s)
	$(COMPOSE_ENV) docker compose run --rm \
	  -e LOCUST_HEADLESS=true \
	  locust locust \
	    -f locustfile.py \
	    --host https://fioserver:8443 \
	    --headless \
	    -u $(NUM_DEVICES) \
	    -r 80 \
	    -t 5m \
	    --csv /data/locust-results \
	    --html /data/locust-report.html \
	    --num-devices $(NUM_DEVICES)

clean: ## Tear down containers and delete seeded data volume (required before re-running)
	-docker compose down -v
	rm -rf $(DATA_DIR)
