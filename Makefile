BUILDFLAGS ?=
unexport GOFLAGS

# Add OSDCTL `./bin/` dir to path for goreleaser
# This will look for goreleaser first in the local bin dir
# but otherwise can be installed elsewhere on developers machines
export PATH := bin:$(PATH)

all: format mod build test

format: vet fmt mockgen ci-build docs

fmt:
	@echo "gofmt"
	@gofmt -w -s .
	@git diff --exit-code .

# https://stackoverflow.com/a/324782
ROOT_DIR:=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

OS := $(shell go env GOOS | sed 's/[a-z]/\U&/')
.PHONY: download-goreleaser
download-goreleaser:
	mkdir -p ./bin && curl -sSLf https://github.com/goreleaser/goreleaser/releases/latest/download/goreleaser_${OS}_x86_64.tar.gz -o - | tar --extract --gunzip --directory ./bin goreleaser

# CI build containers don't include goreleaser by default,
# so they need to get it first, and then run the build
.PHONY: ci-build
ci-build: download-goreleaser build

# Need to use --snapshot here because the goReleaser
# requires more git info that is provided in Prow's clone.
# Snapshot allows the build without validation of the
# repository itself
build:
	${ROOT_DIR}/bin/goreleaser build --rm-dist --snapshot

release:
	${ROOT_DIR}/bin/goreleaser release --rm-dist

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
	go get github.com/golang/mock/mockgen@v1.4.4

test:
	go test ${BUILDFLAGS} ./... -covermode=atomic -coverpkg=./...
