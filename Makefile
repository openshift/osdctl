REPOSITORY = $(shell go list -m)
GIT_COMMIT = $(shell git rev-parse --short HEAD)

BUILDFLAGS ?=
LDFLAGS = -ldflags="-X '${REPOSITORY}/cmd.GitCommit=${GIT_COMMIT}'"
unexport GOFLAGS

all: format build test

format: vet fmt mockgen docs

fmt:
	@echo "gofmt"
	@gofmt -w -s .
	@git diff --exit-code .

build: mod
	go build ${BUILDFLAGS} ${LDFLAGS} -o ./bin/osdctl main.go

vet:
	go vet ${BUILDFLAGS} ./...

mod:
	go mod tidy
	@git diff --exit-code -- go.mod

docs: build
	./bin/osdctl docs ./docs/command
	@git diff --exit-code -- ./docs/command/

mockgen: ensure-mockgen
	go generate ${BUILDFLAGS} ./...
	@git diff --exit-code -- ./pkg/provider/aws/mock

ensure-mockgen:
	go get github.com/golang/mock/mockgen@v1.4.4

test:
	go test ${BUILDFLAGS} ./... -covermode=atomic -coverpkg=./...
