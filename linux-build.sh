#!/usr/bin/env bash
#
# build the Linux artifacts for amd64, arm, arm64
#
# see instructions to run reproducible build with Docker in ./Dockerfile.linux-build
#
set -o pipefail -e -u

GOX_OUTPUT="release/{{.Arch}}/{{.OS}}/{{.Dir}}"
CGO_ENABLED=true gox -os=linux -arch=amd64 -output=${GOX_OUTPUT} ./...
CC=arm-linux-gnueabihf-gcc CGO_ENABLED=true gox -cgo -os=linux -arch=arm -output=${GOX_OUTPUT} ./...
CC=aarch64-linux-gnu-gcc CGO_ENABLED=true gox -cgo -os=linux -arch=arm64 -output=$GOX_OUTPUT ./...
