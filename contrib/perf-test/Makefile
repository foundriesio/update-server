# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

NUM_DEVICES ?= 5000
DATA_DIR    ?= ./perf-test-data

.PHONY: run setup headless clean

setup: ## Generate certs + seed auth only (no load test)
	NUM_DEVICES=$(NUM_DEVICES) DATA_DIR=$(DATA_DIR) docker compose up --build setup

run: ## Web UI at http://localhost:8089 (runs setup first via compose depends_on)
	NUM_DEVICES=$(NUM_DEVICES) DATA_DIR=$(DATA_DIR) docker compose up --build

headless: ## Headless run with CSV+HTML report (spawn-rate capped at 80/s)
	NUM_DEVICES=$(NUM_DEVICES) DATA_DIR=$(DATA_DIR) docker compose run --rm \
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
