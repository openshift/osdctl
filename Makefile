BUILDFLAGS ?=
unexport GOFLAGS

# Add OSDCTL `./bin/` dir to path for goreleaser
# This will look for goreleaser first in the local bin dir
# but otherwise can be installed elsewhere on developers machines
BASE_DIR=$(shell pwd)
export PATH:=$(BASE_DIR)/bin:$(PATH)
SHELL := env PATH=$(PATH) /bin/bash


all: format mod build test

format: vet mod fmt mockgen ci-build docs

fmt:
	@echo "gofmt"
	@gofmt -w -s .
	@git diff --exit-code .

OS := $(shell go env GOOS | sed 's/[a-z]/\U&/')
ARCH := $(shell go env GOARCH)
.PHONY: download-goreleaser
download-goreleaser:
	GOBIN=${BASE_DIR}/bin/ go install github.com/goreleaser/goreleaser@v1.6.3

# CI build containers don't include goreleaser by default,
# so they need to get it first, and then run the build
.PHONY: ci-build
ci-build: download-goreleaser build

SINGLE_TARGET ?= false

# Need to use --snapshot here because the goReleaser
# requires more git info that is provided in Prow's clone.
# Snapshot allows the build without validation of the
# repository itself
build:
	goreleaser build --rm-dist --snapshot --single-target=${SINGLE_TARGET}

release:
	./bin/goreleaser release --rm-dist

vet:
	go vet ${BUILDFLAGS} ./...

mod:
	go mod tidy
	@git diff --exit-code -- go.mod

docs:
	./dist/osdctl_$(shell  uname | tr [:upper:] [:lower:])_amd64/osdctl ./docs/command
	@git diff --exit-code -- ./docs/command/

mockgen: ensure-mockgen
	go generate ${BUILDFLAGS} ./...
	@git diff --exit-code -- ./pkg/provider/aws/mock

ensure-mockgen:
	GOBIN=${BASE_DIR}/bin/  go install github.com/golang/mock/mockgen@v1.6.0

test:
	go test ${BUILDFLAGS} ./... -covermode=atomic -coverpkg=./...
