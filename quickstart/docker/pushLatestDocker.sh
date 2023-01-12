#!/usr/bin/env bash
set -eo pipefail

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

ZITI_QUICKSTART_ROOT="$(realpath ${SCRIPT_DIR}/..)"
ZITI_BIN_ROOT="${ZITI_QUICKSTART_ROOT}/docker"

for arch in amd64 arm64; do
  (export ZITI_OSTYPE=linux; export ZITI_ARCH="${arch}"
   source "${ZITI_QUICKSTART_ROOT}/ziti-cli-functions.sh"
   if [ -d "${ZITI_BIN_ROOT}/image/ziti.ignore" ]; then
     rm -rf "${ZITI_BIN_ROOT}/image/ziti.ignore"
   fi

   getZiti

   mv "${ZITI_BIN_DIR}" "${SCRIPT_DIR}/image/ziti.ignore/"
   docker build --platform linux/${arch} "${SCRIPT_DIR}/image" -t openziti/quickstart

   if [ -d "${ZITI_BIN_ROOT}/image/ziti.ignore" ]; then
     rm -rf "${ZITI_BIN_ROOT}/image/ziti.ignore"
   fi

   if [ -d "${ZITI_BIN_ROOT}/ziti-bin" ]; then
     rm -rf "${ZITI_BIN_ROOT}/ziti-bin"
   fi

   vers="$(echo "${ZITI_BINARIES_VERSION}" | cut -c 2-100)"
   echo DOCKERTAG: docker tag openziti/quickstart "openziti/quickstart:${vers}"
   docker tag openziti/quickstart "openziti/quickstart:${vers}"
   echo DOCKERTAG: docker tag openziti/quickstart "openziti/quickstart:latest"
   docker tag openziti/quickstart "openziti/quickstart:latest"
   echo DOCKERPUSH: docker push "openziti/quickstart:${vers}"
   docker push "openziti/quickstart:${vers}"
   echo DOCKERPUSH: docker push "openziti/quickstart:latest"
   docker push "openziti/quickstart:latest")
done