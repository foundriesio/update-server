COMMIT?=$(shell git describe --tags HEAD)$(shell git diff --quiet || echo '+dirty')

# Use linker flags to provide commit info
LDFLAGS=-ldflags "-X=github.com/foundriesio/update-server/version.Version=$(COMMIT)"

all: fiocli fioserver

build-cli: fiocli-linux-amd64 fiocli-linux-arm64 fiocli-windows-amd64.exe fiocli-windows-arm64.exe fiocli-darwin-arm64 fiocli-darwin-amd64

HOST_OS:=$(shell go env GOOS)
HOST_ARCH:=$(shell go env GOARCH)
HOST_EXT:=$(if $(filter windows,$(HOST_OS)),.exe,)

fiocli: fiocli-$(HOST_OS)-$(HOST_ARCH)$(HOST_EXT)

fioserver:
	go build $(LDFLAGS) -o bin/$@ github.com/foundriesio/update-server/cmd/server

fiocli-linux-amd64:
fiocli-linux-arm64:
fiocli-windows-amd64.exe:
fiocli-windows-arm64.exe:
fiocli-darwin-amd64:
fiocli-darwin-arm64:
fiocli-%:
	CGO_ENABLED=0 \
	GOOS=$(shell echo $* | cut -f1 -d\- ) \
	GOARCH=$(shell echo $* | cut -f2 -d\- | cut -f1 -d. ) \
		go build $(LDFLAGS) -tags nodb -o bin/$@ github.com/foundriesio/update-server/cmd/cli

swagger: swagger-api swagger-gateway

swagger-api:
	swag init --parseDependency --parseInternal \
		-d ./server/ui/api \
		-g doc.go \
		-o ./docs/swagger/api \
		--outputTypes json,yaml \
		--instanceName api

swagger-gateway:
	swag init --parseDependency --parseInternal \
		-d ./server/gateway \
		-g doc.go \
		-o ./docs/swagger/gateway \
		--outputTypes json,yaml \
		--instanceName gateway

# Languages from LINGUAS: comments dropped, `_` read as `-`, en is the source.
I18N_DIR := server/ui/i18n
I18N_TEMPLATES ?= server/ui/web/templates
# native writes the POT itself; xgettext routes it through GNU xgettext 1.0.
I18N_EXTRACTOR ?= native
HASH := \#
I18N_ALL := $(shell sed -e 's/$(HASH).*//' -e 's/_/-/g' $(I18N_DIR)/LINGUAS)
I18N_LANGS := $(filter-out en,$(I18N_ALL))

.PHONY: i18n-gettext i18n-extract i18n-merge i18n-compile i18n-gate i18n-check

# msgmerge before 1.0 drops #, go-format flags, so merge and compile need 1.0.
i18n-gettext:
	@v=$$(msgmerge --version | sed -n '1s/.* //p'); \
	case "$$v" in 0.*|'') echo "i18n: GNU gettext >= 1.0 required, found '$$v'" >&2; exit 1;; esac

i18n-extract:
	go run ./internal/tools/i18nscan -templates $(I18N_TEMPLATES) -go auth,server/ui/web -root . -extractor $(I18N_EXTRACTOR) -out $(I18N_DIR)/po/ui.pot

i18n-merge: i18n-gettext
	for lang in $(I18N_LANGS); do \
		msgmerge --update --backup=none $(I18N_DIR)/po/$$lang.po $(I18N_DIR)/po/ui.pot || exit 1; \
	done

i18n-compile: i18n-gettext
	for lang in $(I18N_LANGS); do \
		mkdir -p $(I18N_DIR)/locale/$$lang/LC_MESSAGES && \
		msgfmt --check --output-file=$(I18N_DIR)/locale/$$lang/LC_MESSAGES/ui.mo $(I18N_DIR)/po/$$lang.po || exit 1; \
	done

# Counts msgid lines rather than gating on output: the header entry may or may
# not be printed, and a wrapped msgid does not match ^msgid ".+".
i18n-gate:
	test -n "$(PO)" && test -f "$(PO)" || { echo "i18n-gate: PO=<file> is required" >&2; exit 1; }
	for flag in --untranslated --only-fuzzy; do \
		left=$$(msgattrib $$flag --no-obsolete $(PO)) || exit 1; \
		if [ "$$(printf '%s\n' "$$left" | grep -c '^msgid ')" -gt 1 ]; then echo "$(PO): $$flag messages remain" >&2; exit 1; fi; \
	done

i18n-check:
	test -n "$(I18N_ALL)" || { echo "$(I18N_DIR)/LINGUAS has no language tags" >&2; exit 1; }
	for lang in $(I18N_LANGS); do \
		test -f $(I18N_DIR)/po/$$lang.po || { echo "$$lang: missing $(I18N_DIR)/po/$$lang.po, create it with msginit (see CONTRIBUTING.md)" >&2; exit 1; }; \
	done
	$(MAKE) i18n-extract
	$(MAKE) i18n-merge
	$(MAKE) i18n-compile
	test -z "$$(git status --porcelain -- $(I18N_DIR))" || { git status --short -- $(I18N_DIR); exit 1; }
	for lang in $(I18N_LANGS); do \
		$(MAKE) --no-print-directory i18n-gate PO=$(I18N_DIR)/po/$$lang.po || exit 1; \
	done
	go test ./server/ui/i18n/... ./internal/tools/i18nscan/...

