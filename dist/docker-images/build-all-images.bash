#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

# cd to project root
DIRNAME=$(dirname "$0")
cd "$DIRNAME/../.."  # the images can be built from anywhere, but the Dockerfile defaults assume the project root is the build context root

# TODO:
# - add an --arm64 flag to build the ziti binary and container images for arm64
# - add a --namespace option to specify the registry hostname and namespace/org to tag each image, e.g., --namespace 127.0.0.1:5000/localtest
# - add a --push flag to push the images to the registry instead of loading them into the build context

# define a version based on the most recent tag
ZITI_VERSION=$(git describe --tags --always)

: build the go build env
docker buildx build \
    --tag=ziti-go-builder \
    --build-arg uid=$UID \
    --load \
    ./dist/docker-images/cross-build/

: build the ziti binary for amd64 in ARTIFACTS_DIR/TARGETARCH/TARGETOS/ziti
docker run \
    --rm \
    --user "$UID" \
    --name=ziti-go-builder \
    --volume=$PWD:/mnt \
    --volume=${GOCACHE:-${HOME}/.cache/go-build}:/.cache/go-build \
    --env=GOCACHE=/.cache/go-build \
    ziti-go-builder amd64

: build the cli image with binary from ARTIFACTS_DIR/TARGETARCH/TARGETOS/ziti
docker buildx build \
  --platform=linux/amd64 \
  --tag "ziti-cli:${ZITI_VERSION}" \
  --file ./dist/docker-images/ziti-cli/Dockerfile \
  --load \
  $PWD 

docker build \
  --build-arg ZITI_CLI_IMAGE="ziti-cli" \
  --build-arg ZITI_CLI_TAG="${ZITI_VERSION}" \
  --platform=linux/amd64 \
  --tag "ziti-controller:${ZITI_VERSION}" \
  --file ./dist/docker-images/ziti-controller/Dockerfile \
  --load \
  $PWD

docker build \
  --build-arg ZITI_CLI_IMAGE="ziti-cli" \
  --build-arg ZITI_CLI_TAG="${ZITI_VERSION}" \
  --platform=linux/amd64 \
  --tag "ziti-router:${ZITI_VERSION}" \
  --file ./dist/docker-images/ziti-router/Dockerfile \
  --load \
  $PWD

docker build \
  --build-arg ZITI_CLI_IMAGE="ziti-cli" \
  --build-arg ZITI_CLI_TAG="${ZITI_VERSION}" \
  --platform=linux/amd64 \
  --tag "ziti-tunnel:${ZITI_VERSION}" \
  --file ./dist/docker-images/ziti-tunnel/Dockerfile \
  --load \
  $PWD
