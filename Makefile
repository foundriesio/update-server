.PHONY: docker-build download run clean

CACHE := .cache

docker-build:
	docker build -t dg-sat-e2e-tools .

download: docker-build $(CACHE)/dg-sat $(CACHE)/fioup.deb $(CACHE)/debian-trixie.qcow2 $(CACHE)/composectl

$(CACHE)/dg-sat:
	mkdir -p $(CACHE)
	curl -fsSL -o $@ \
	  https://github.com/foundriesio/dg-satellite/releases/download/v0.7/dg-sat-linux-amd64
	chmod +x $@

$(CACHE)/fioup.deb:
	mkdir -p $(CACHE)
	curl -fsSL -o $@ \
	  https://github.com/foundriesio/fioup/releases/download/v1.3.3/fioup_1.3.3_amd64.deb

$(CACHE)/debian-trixie.qcow2:
	mkdir -p $(CACHE)
	curl -fL -o $@ \
	  https://cloud.debian.org/images/cloud/trixie/latest/debian-13-genericcloud-amd64.qcow2

$(CACHE)/composectl:
	mkdir -p $(CACHE)
	curl -fsSL -o $@ \
	  https://github.com/foundriesio/composeapp/releases/download/v96.2.1/composectl_96.2.1_linux_amd64
	chmod +x $@

venv: .venv/bin/activate
.venv/bin/activate: requirements.txt
	python3 -m venv .venv
	.venv/bin/pip install -r requirements.txt
	.venv/bin/playwright install chromium

run: download venv
	.venv/bin/pytest -s -v test_connection.py

clean:
	rm -rf .cache __pycache__ .pytest_cache
