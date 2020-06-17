GOOS := $(if $(GOOS),$(GOOS),linux)
GOARCH := $(if $(GOARCH),$(GOARCH),amd64)
GO=GO15VENDOREXPERIMENT="1" CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) GO111MODULE=on go

FILES_TO_FMT  := $(shell find . -path -prune -o -name '*.go' -print)

all: format build

format: vet fmt

fmt:
	@echo "gofmt"
	@gofmt -w ${FILES_TO_FMT}
	@git diff --exit-code .

build: mod
	go build -o ./bin/osd-utils-cli main.go

vet:
	go vet ./...

mod:
	@echo "go mod tidy"
	GO111MODULE=on go mod tidy
	@git diff --exit-code -- go.mod

docgen: build-docgen
	./bin/docgen ./docs/command

build-docgen:
	go build -o ./bin/docgen docgen/main.go

check-docs:
	@make docgen
	@git diff --exit-code -- docs/command
