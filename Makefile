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

all: format mod ci-build test lint verify-docs

format: vet mod fmt mockgen ## Runs vet, mod, fmt & mockgen targets

fmt: ## format go code
	@echo "gofmt"
	@gofmt -w -s .
	@git diff --exit-code .

OS := $(shell go env GOOS | sed 's/[a-z]/\U&/')
ARCH := $(shell go env GOARCH)
GORELEASER_VERSION_FILE := .goreleaser-version
GORELEASER_VERSION := $(shell test -f ${GORELEASER_VERSION_FILE} && cat ${GORELEASER_VERSION_FILE})

.PHONY: download-goreleaser
download-goreleaser: check-goreleaser-version-file ## Download goreleaser binary
	GOBIN=${BASE_DIR}/bin/ go install github.com/goreleaser/goreleaser/v2@${GORELEASER_VERSION}

# Update documentation as a part of every release
.PHONY: generate-docs
generate-docs: ## Generate docs
	@go run utils/docgen/main.go --cmd-path=./cmd --docs-dir=./docs
	
# Verify documents using PROW as a part of every PR raised for osdctl
.PHONY: verify-docs
verify-docs:
	./scripts/verify-docs.sh

.PHONY: ci-build
ci-build: check-goreleaser ## Build for CI environment
	$(MAKE) build

.PHONY: check-goreleaser
check-goreleaser: check-goreleaser-version-file ## Verify the goreleaser version
	@if ! command -v goreleaser >/dev/null; then \
		echo "goreleaser is not installed; run 'make download-goreleaser'." >&2; \
		exit 1; \
	fi
	@actual_version="$$(goreleaser --version | awk '/^GitVersion:/ { print $$2; exit }')"; \
	if [ "v$${actual_version#v}" != "${GORELEASER_VERSION}" ]; then \
		echo "goreleaser version $$actual_version does not match required ${GORELEASER_VERSION}; run 'make download-goreleaser'." >&2; \
		exit 1; \
	fi

.PHONY: check-goreleaser-version-file
check-goreleaser-version-file: ## Verify the configured goreleaser version
	@if [ ! -f "${GORELEASER_VERSION_FILE}" ]; then \
		echo "${GORELEASER_VERSION_FILE} is missing; restore it with the required goreleaser version." >&2; \
		exit 1; \
	fi
	@if ! echo "${GORELEASER_VERSION}" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+$$'; then \
		echo "${GORELEASER_VERSION_FILE} must contain a vMAJOR.MINOR.PATCH version, got '${GORELEASER_VERSION}'." >&2; \
		exit 1; \
	fi

SINGLE_TARGET ?= false

# Need to use --snapshot here because goreleaser
# requires more git info than what is provided in Prow's clone.
# Snapshot allows the build without validation of the
# repository itself
build: ## Compile osdctl
	goreleaser build --clean --snapshot --single-target=${SINGLE_TARGET}

# Preferred way to cut a release: bump the VERSION file on a branch and open a PR.
# Once merged to master, GitHub Actions tags vX.Y.Z and publishes the release
# (and triggers the Fedora COPR build). See README "Creating a release".
.PHONY: new-release
new-release: ## Bump VERSION for a new release (RELEASE_VERSION=x.y.z), then open a PR
	@if [ -z "$(RELEASE_VERSION)" ]; then \
		echo "Usage: make new-release RELEASE_VERSION=x.y.z (current: $$(cat VERSION))"; \
		exit 1; \
	fi
	@if ! echo "$(RELEASE_VERSION)" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$$'; then \
		echo "RELEASE_VERSION must be MAJOR.MINOR.PATCH (e.g. 0.63.0), got '$(RELEASE_VERSION)'"; \
		exit 1; \
	fi
	@echo "$(RELEASE_VERSION)" > VERSION
	@echo "Updated VERSION -> $(RELEASE_VERSION)."
	@echo "Commit this on a branch and open a PR. Merging it tags v$(RELEASE_VERSION) and publishes the release."

# Manual fallback: build and publish a release from your machine with goreleaser.
# Normally the 'release' GitHub Action does this on tag push; use this only if the
# Action is unavailable. Requires a GitHub token in ~/.config/goreleaser/token.
release: ## Create a release locally (manual fallback; prefer 'make new-release')
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
