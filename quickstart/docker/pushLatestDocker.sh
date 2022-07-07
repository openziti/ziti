#!/usr/bin/env bash
set -eo pipefail

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

ZITI_QUICKSTART_ROOT="$(realpath ${SCRIPT_DIR}/..)"
ZITI_BIN_ROOT="${ZITI_QUICKSTART_ROOT}/docker"

mkdir -p "${ZITI_BIN_ROOT}/ziti-bin"

source "${ZITI_QUICKSTART_ROOT}/ziti-cli-functions.sh"

ZITI_HOME="${ZITI_BIN_ROOT}"

if [ -d "${ZITI_BIN_ROOT}/image/ziti.ignore" ]; then
  rm -rf "${ZITI_BIN_ROOT}/image/ziti.ignore"
fi

getZiti

mv "${ZITI_BIN_DIR}" "${SCRIPT_DIR}/image/ziti.ignore/"
docker build "${SCRIPT_DIR}/image" -t openziti/quickstart

if [ -d "${ZITI_BIN_ROOT}/image/ziti.ignore" ]; then
  rm -rf "${ZITI_BIN_ROOT}/image/ziti.ignore"
fi

docker run --rm -it openziti/quickstart /openziti/ziti-bin/ziti version
#if [ -f "${ZITI_BIN_ROOT}/ziti-*tar.gz" ]; then
#  rm "${ZITI_BIN_ROOT}/ziti-*tar.gz"
#fi
if [ -d "${ZITI_BIN_ROOT}/ziti-bin" ]; then
  rm -rf "${ZITI_BIN_ROOT}/ziti-bin"
fi
if [ -d "${ZITI_BIN_ROOT}/image/ziti.ignore" ]; then
  rm -rf "${ZITI_BIN_ROOT}/image/ziti.ignore/"
fi

vers="$(echo "${ZITI_BINARIES_VERSION}" | cut -c 2-100)"
echo DOCKERTAG: docker tag openziti/quickstart "openziti/quickstart:${vers}"
docker tag openziti/quickstart "openziti/quickstart:${vers}"
echo DOCKERTAG: docker tag openziti/quickstart "openziti/quickstart:latest"
docker tag openziti/quickstart "openziti/quickstart:latest"
echo DOCKERPUSH: docker push "openziti/quickstart:${vers}"
docker push "openziti/quickstart:${vers}"
ecoh DOCKERPUSH: docker push "openziti/quickstart:latest"
docker push "openziti/quickstart:latest"
