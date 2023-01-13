#!/usr/bin/env bash
set -eo pipefail

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

if [ -z "${ZITI_VERSION}" ]; then
  ZITI_QUICKSTART_ROOT="$(realpath ${SCRIPT_DIR}/..)"
  v=$(source "${ZITI_QUICKSTART_ROOT}/ziti-cli-functions.sh"; getLatestZitiVersion > /dev/null 2>&1; echo ${ZITI_BINARIES_VERSION})
  ZITI_VERSION=$(echo "${v}" | sed -e 's/^v//')
fi

if [ -z "${ZITI_VERSION}" ]; then
  echo "ZITI_VERSION was not set and auto-detection failed."
  exit 1
fi

docker buildx create --use --name=ziti-builder
docker buildx build --platform linux/amd64,linux/arm64 "${SCRIPT_DIR}/image" \
  --build-arg ZITI_VERSION="${ZITI_VERSION}" \
  --tag "openziti/quickstart:${ZITI_VERSION}" \
  --tag "openziti/quickstart:latest" \
  --push
