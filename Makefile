COMMIT?=$(shell git describe --tags HEAD)$(shell git diff --quiet || echo '+dirty')

# Use linker flags to provide commit info
LDFLAGS=-ldflags "-X=github.com/foundriesio/update-server/version.Version=$(COMMIT)"

build-cli: fiocli-linux-amd64 fiocli-linux-arm64 fiocli-windows-amd64.exe fiocli-windows-arm64.exe fiocli-darwin-arm64 fiocli-darwin-amd64

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

