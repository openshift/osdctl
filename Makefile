BUILDFLAGS ?=
GORELEASER_BUILD_ARGS = "--rm-dist"
unexport GOFLAGS

all: format mod build test

format: vet fmt mockgen ci-build docs

fmt:
	@echo "gofmt"
	@gofmt -w -s .
	@git diff --exit-code .

.PHONY: download-goreleaser
download-goreleaser:
	mkdir -p ./bin && curl -sSLf https://github.com/goreleaser/goreleaser/releases/latest/download/goreleaser_Linux_x86_64.tar.gz -o - | tar --extract --gunzip --directory ./bin goreleaser

# Need to use --snapshot here because the goReleaser
# requires more git info that is provided in Prow's clone.
# Snapshot allows the build without validation of the
# repository itself
.PHONY: ci-build
ci-build: download-goreleaser
	./bin/goreleaser build $(GORELEASER_BUILD_ARGS) --snapshot

build:
	goreleaser build $(GORELEASER_BUILD_ARGS)

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
