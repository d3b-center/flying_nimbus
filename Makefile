
BINARY_NAME := flying-nimbus
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT_HASH := $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
PLATFORM := $(shell uname -s | tr '[:upper:]' '[:lower:]')

LDFLAGS := -s -w \
	-X flying_nimbus/cmd.Version=$(VERSION) \
	-X flying_nimbus/cmd.CommitHash=$(COMMIT_HASH) \
	-X flying_nimbus/cmd.Branch=$(BRANCH) \
	-X flying_nimbus/cmd.BuildDate=$(BUILD_DATE) \
	-X flying_nimbus/cmd.Platform=$(PLATFORM)

## Run
run:
	go run .

clean:
	go clean -cache
	go clean -modcache
	rm -f $(BINARY_NAME)

tidy:
	go mod tidy

## Build Binary
build:
	CGO_ENABLED=0 go build \
	-trimpath \
	-ldflags '$(LDFLAGS)' \
	-o $(BINARY_NAME)
test:
	go test ./...
