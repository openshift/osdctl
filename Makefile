BUILDFLAGS ?=
unexport GOFLAGS

# Add OSDCTL `./bin/` dir to path for goreleaser
# This will look for goreleaser first in the local bin dir
# but otherwise can be installed elsewhere on developers machines
BASE_DIR=$(shell pwd)
export PATH:=$(BASE_DIR)/bin:$(PATH)
SHELL := /bin/bash

.DEFAULT_GOAL := all

.PHONY: help
help:
	@echo "================================================"
	@echo "              osdctl Makefile Help              "
	@echo "================================================"
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

all: format mod build test lint verify-docs

format: vet mod fmt mockgen ci-build ## Runs vet, mod, fmt, mockgen & ci-build targets

fmt: ## format go code
	@echo "gofmt"
	@gofmt -w -s .
	@git diff --exit-code .

OS := $(shell go env GOOS | sed 's/[a-z]/\U&/')
ARCH := $(shell go env GOARCH)
.PHONY: download-goreleaser
download-goreleaser: ## Download goreleaser binary
	GOBIN=${BASE_DIR}/bin/ go install github.com/goreleaser/goreleaser/v2@v2.11.2

# Update documentation as a part of every release
.PHONY: generate-docs
generate-docs: ## Generate docs
	@go run utils/docgen/main.go --cmd-path=./cmd --docs-dir=./docs
	
# Verify documents using PROW as a part of every PR raised for osdctl
.PHONY: verify-docs
verify-docs:
	./scripts/verify-docs.sh

# CI build containers don't include goreleaser by default,
# so they need to get it first, and then run the build
.PHONY: ci-build
ci-build: download-goreleaser build ## Build for CI environment

SINGLE_TARGET ?= false

# Need to use --snapshot here because goreleaser
# requires more git info than what is provided in Prow's clone.
# Snapshot allows the build without validation of the
# repository itself
build: ## Compile osdctl
	goreleaser build --clean --snapshot --single-target=${SINGLE_TARGET}

release: ## Create a release
	goreleaser release --clean

install: ## Install osdctl to GOPATH/bin
	goreleaser build --single-target -o "$(shell go env GOPATH)/bin/osdctl" --snapshot --clean

vet: ## Run go vet
	go vet ${BUILDFLAGS} ./...

mod: ## Tidy go modules
	go mod tidy
	@git diff --exit-code -- go.mod

mockgen: ensure-mockgen ## Generate mocks
	go generate ${BUILDFLAGS} ./...
	@git diff --exit-code -- ./pkg/provider/aws/mock

ensure-mockgen: ## Install mockgen dependency
	GOBIN=${BASE_DIR}/bin/ go install go.uber.org/mock/mockgen@v0.6.0

test: ## Run tests
	go test ${BUILDFLAGS} ./... -covermode=atomic -coverpkg=./...

lint: ## Run linter
	golangci-lint run
