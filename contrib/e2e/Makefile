.PHONY: fioup-vm build clean

CACHE := .cache

build: fioup-e2e $(CACHE)/fioserver $(CACHE)/fiocli $(CACHE)/composectl venv

fioup-e2e: $(CACHE)/fioup-e2e.stamp
$(CACHE)/fioup-e2e.stamp: Dockerfile.fioup
	mkdir -p $(CACHE)
	docker build -t fioup-e2e -f Dockerfile.fioup .
	touch $@

$(CACHE)/fioserver:
	mkdir -p $(CACHE)
	make -C ../.. fioserver
	cp ../../bin/fioserver $(CACHE)/fioserver

$(CACHE)/fiocli:
	mkdir -p $(CACHE)
	make -C ../.. fiocli-linux-amd64
	cp ../../bin/fiocli-linux-amd64 $(CACHE)/fiocli

$(CACHE)/composectl:
	mkdir -p $(CACHE)
	curl -fsSL -o $@ \
	  https://github.com/foundriesio/composeapp/releases/download/v96.2.1/composectl_96.2.1_linux_amd64
	chmod +x $@

venv: $(CACHE)/venv/bin/activate
$(CACHE)/venv/bin/activate: requirements.txt
	mkdir -p $(CACHE)
	python3 -m venv $(CACHE)/venv
	$(CACHE)/venv/bin/pip install -r requirements.txt
	$(CACHE)/venv/bin/playwright install chromium

run: build
	$(CACHE)/venv/bin/pytest -s -v test_connection.py
	$(CACHE)/venv/bin/pytest -s -v test_fiocli.py
	$(CACHE)/venv/bin/pytest -s -v test_remote_actions.py
	$(CACHE)/venv/bin/pytest -s -v test_updates.py
	$(CACHE)/venv/bin/pytest -s -v test_webui.py
	$(CACHE)/venv/bin/pytest -s -v test_webui_settings.py

clean:
	rm -rf .cache 
