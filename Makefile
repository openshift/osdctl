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

OS := $(shell go env GOOS)
.PHONY: download-goreleaser
download-goreleaser:
	@if [ "${OS}" = "darwin" ]; then\
		mkdir -p ./bin && curl -sSLf https://github.com/goreleaser/goreleaser/releases/latest/download/goreleaser_Darwin_all.tar.gz -o - | tar --extract --gunzip --directory ./bin goreleaser;\
	fi
	@if [ "${OS}" = "linux" ]; then\
    		mkdir -p ./bin && curl -sSLf https://github.com/goreleaser/goreleaser/releases/latest/download/goreleaser_Linux_x86_64.tar.gz -o - | tar --extract --gunzip --directory ./bin goreleaser;\
    fi

# CI build containers don't include goreleaser by default,
# so they need to get it first, and then run the build
.PHONY: ci-build
ci-build: download-goreleaser build

# Need to use --snapshot here because the goReleaser
# requires more git info that is provided in Prow's clone.
# Snapshot allows the build without validation of the
# repository itself
build:
	goreleaser build --rm-dist --snapshot

release:
	goreleaser release --rm-dist

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
