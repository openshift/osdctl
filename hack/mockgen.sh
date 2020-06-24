#!/usr/bin/env bash

set -eu

MOCKGEN_BIN=${PROTOC_BIN:-mockgen}

if ! [[ "$0" =~ "hack/mockgen.sh" ]]; then
	echo "must be run from repository root"
	exit 255
fi

mockgen -source=pkg/provider/aws/client.go -package=mock -destination=pkg/provider/aws/mock/client.go
