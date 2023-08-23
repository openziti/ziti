#!/usr/bin/env bash
set -eo pipefail

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

if [ -z "${ZITI_VERSION}" ]; then
  DOCKER_IMAGE_ROOT="$(realpath ${SCRIPT_DIR}/image)"
  v=$(source "${DOCKER_IMAGE_ROOT}/ziti-cli-functions.sh"; getLatestZitiVersion > /dev/null 2>&1; echo ${ZITI_BINARIES_VERSION})
  ZITI_VERSION=$(echo "${v}" | sed -e 's/^v//')
  echo "ZITI_VERSION=${ZITI_VERSION}"
fi

if [ -z "${ZITI_VERSION}" ]; then
  echo "ZITI_VERSION was not set and auto-detection failed."
  exit 1
fi

IMAGE_TAG="${2-}"
if [ -z "${IMAGE_TAG}" ]; then
  IMAGE_TAG="latest"
  echo "image tag name was not provided, using default '${IMAGE_TAG}'"
fi

if [ "local" == "${1}" ]; then
  echo "LOADING LOCALLY instead of pushing to dockerhub"
  _BUILDX_PLATFORM=""
  _BUILDX_ACTION="--load"
else
  _BUILDX_PLATFORM="--platform linux/amd64,linux/arm64"
  _BUILDX_ACTION="--push"
fi

docker buildx create \
  --use --name=ziti-builder --driver docker-container 2>/dev/null \
  || docker buildx use --default ziti-builder

eval docker buildx build "${_BUILDX_PLATFORM}" "${SCRIPT_DIR}/image" \
  --build-arg ZITI_VERSION_OVERRIDE="v${ZITI_VERSION}" \
  --tag "openziti/quickstart:${ZITI_VERSION}" \
  --tag "openziti/quickstart:${IMAGE_TAG}" \
  "${_BUILDX_ACTION}"
