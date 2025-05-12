BUILDFLAGS ?=
unexport GOFLAGS

# Add OSDCTL `./bin/` dir to path for goreleaser
# This will look for goreleaser first in the local bin dir
# but otherwise can be installed elsewhere on developers machines
BASE_DIR=$(shell pwd)
export PATH:=$(BASE_DIR)/bin:$(PATH)
SHELL := /bin/bash


all: format mod build test lint

format: vet mod fmt mockgen ci-build

fmt:
	@echo "gofmt"
	@gofmt -w -s .
	@git diff --exit-code .

OS := $(shell go env GOOS | sed 's/[a-z]/\U&/')
ARCH := $(shell go env GOARCH)
.PHONY: download-goreleaser
download-goreleaser:
	GOBIN=${BASE_DIR}/bin/ go install github.com/goreleaser/goreleaser/v2@v2.6.1 # TODO: bump once we move to Go 1.24

#Update documentation as a part of every release

.PHONY: generate-docs
generate-docs:
	@go run utils/docgen/main.go --cmd-path=./cmd --docs-dir=./docs
	
#Verify documents using PROW as a part of every PR raised for osdctl

.PHONY: verify-docs
verify-docs:
	./scripts/verify-docs.sh

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
	goreleaser build --clean --snapshot --single-target=${SINGLE_TARGET}

release:
	goreleaser release --clean

install:
	goreleaser build --single-target -o "$(shell go env GOPATH)/bin/osdctl" --snapshot --clean

vet:
	go vet ${BUILDFLAGS} ./...

mod:
	go mod tidy
	@git diff --exit-code -- go.mod

mockgen: ensure-mockgen
	go generate ${BUILDFLAGS} ./...
	@git diff --exit-code -- ./pkg/provider/aws/mock

ensure-mockgen:
	GOBIN=${BASE_DIR}/bin/ go install go.uber.org/mock/mockgen@v0.5.0

test:
	go test ${BUILDFLAGS} ./... -covermode=atomic -coverpkg=./...

lint:
	golangci-lint run
