.PHONY: fioup-vm build aklite-build aklite-run run clean

CACHE := .cache

# Path to a checkout of https://github.com/foundriesio/aktualizr-lite, used to
# build the aktualizr-lite binary and to source make_sys_rootfs.sh.
AKLITE_REPO ?= $(abspath ../../../aktualizr-lite)

# Base image aktualizr-lite is built in. Pinned to match Dockerfile.aklite's base
# so the staged binary links against the same runtime libraries. The engine path
# is passed unquoted (the aklite dev-flow.mk default embeds quotes, which breaks
# CMake's path existence check).
AKLITE_DEV_IMAGE ?= foundries/aklite-dev:ubuntu-25.04
PKCS11_ENGINE_PATH ?= /usr/lib/x86_64-linux-gnu/engines-3/pkcs11.so

build: fioup-e2e $(CACHE)/fioserver $(CACHE)/fiocli $(CACHE)/composectl venv

# The aktualizr-lite e2e build is kept separate from `build` because it requires
# a local aktualizr-lite checkout (AKLITE_REPO) and builds it from source in a
# container. Run it explicitly (e.g. `make aklite-build`) where that toolchain
# is available; the default `build` used by CI only provisions the fioup flow.
aklite-build: aklite-e2e $(CACHE)/fioserver $(CACHE)/fiocli $(CACHE)/composectl venv

fioup-e2e: $(CACHE)/fioup-e2e.stamp
$(CACHE)/fioup-e2e.stamp: Dockerfile.fioup
	mkdir -p $(CACHE)
	docker build -t fioup-e2e -f Dockerfile.fioup .
	touch $@

# Build the aktualizr-lite binary via the aklite repo's own containerized flow,
# then stage it (and make_sys_rootfs.sh) into .cache/ so the image links against
# the same aklite-dev base it was built on.
$(CACHE)/aktualizr-lite:
	mkdir -p $(CACHE)
	cd $(AKLITE_REPO) && $(MAKE) all TARGET=aktualizr-lite BUILD_DIR=build \
		CONTAINER=$(AKLITE_DEV_IMAGE) PKCS11_ENGINE_PATH=$(PKCS11_ENGINE_PATH)
	cp $(AKLITE_REPO)/build/src/aktualizr-lite $(CACHE)/aktualizr-lite
	cp $(AKLITE_REPO)/build/src/libaktualizr_lite.so $(CACHE)/libaktualizr_lite.so
	cp $(AKLITE_REPO)/build/aktualizr/src/libaktualizr/libaktualizr.so $(CACHE)/libaktualizr.so

# Build fiopull from source using a throwaway Go container.
$(CACHE)/fiopull:
	mkdir -p $(CACHE)
	docker run --rm -v $(CACHE):/out golang:1.24 sh -c "\
		git clone --depth=1 https://github.com/foundriesio/ostreeuploader.git /src && \
		cd /src && make fiopull && cp bin/fiopull /out/fiopull"

$(CACHE)/make_sys_rootfs.sh: $(AKLITE_REPO)/tests/make_sys_rootfs.sh
	mkdir -p $(CACHE)
	cp $(AKLITE_REPO)/tests/make_sys_rootfs.sh $(CACHE)/make_sys_rootfs.sh

aklite-e2e: $(CACHE)/aklite-e2e.stamp
$(CACHE)/aklite-e2e.stamp: Dockerfile.aklite entrypoint.aklite.sh $(CACHE)/aktualizr-lite $(CACHE)/fiopull $(CACHE)/make_sys_rootfs.sh $(CACHE)/composectl
	docker build -t aklite-e2e -f Dockerfile.aklite .
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
	$(CACHE)/venv/bin/pytest -s -v test_oauth2_login.py
	$(CACHE)/venv/bin/pytest -s -v test_e2e_update_flow.py

aklite-run: aklite-build
	$(CACHE)/venv/bin/pytest -s -v test_e2e_aklite_update_flow.py

clean:
	rm -rf .cache
