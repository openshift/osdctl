#
# Copyright (c) 2020 Red Hat, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#   http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

REPOSITORY = $(shell go list -m)
GIT_COMMIT = $(shell git rev-parse --short HEAD)

# Ensure go modules are enabled:
export GO111MODULE=on
export GOPROXY=https://proxy.golang.org

 Disable CGO so that we always generate static binaries:
export CGO_ENABLED=0

# Unset GOFLAG for CI and ensure we've got nothing accidently set
unexport GOFLAGS

BUILDFLAGS ?=
LDFLAGS = -ldflags="-X '${REPOSITORY}/cmd.GitCommit=${GIT_COMMIT}'"
unexport GOFLAGS

.PHONY: build
build:
	go build ${BUILDFLAGS} ${LDFLAGS} -o ./bin/osdctl main.go

.PHONY: test
test:
	go test ${BUILDFLAGS} ./... -covermode=atomic -coverpkg=./...

.PHONY: clean
	rm -rf ./bin

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

