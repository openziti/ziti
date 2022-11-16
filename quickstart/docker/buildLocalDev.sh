#!/usr/bin/env bash
set -eo pipefail

echo "${SHELL}"

# This method is required for users bash prior to version 4. More specifically this is to support Mac users as Mac uses
# an older version of bash.
myrealpath() {
  OURPWD=$PWD
  cd "$(dirname "$1")"
  LINK=$(readlink "$(basename "$1")")
  while [ "$LINK" ]; do
    cd "$(dirname "$LINK")"
    LINK=$(readlink "$(basename "$1")")
  done
  REALPATH="$PWD/$(basename "$1")"
  cd "$OURPWD"
  echo "$REALPATH"
}

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
if [ "${BASH_VERSINFO:-0}" -lt 4 ]; then
  ZITI_QUICKSTART_ROOT="$(myrealpath "${SCRIPT_DIR}"/..)"
  ZITI_ROOT="$(myrealpath "${ZITI_QUICKSTART_ROOT}"/..)"
else
  ZITI_QUICKSTART_ROOT="$(realpath "${SCRIPT_DIR}"/..)"
  ZITI_ROOT="$(realpath "${ZITI_QUICKSTART_ROOT}"/..)"
fi
ZITI_DOCKER_ROOT="${ZITI_QUICKSTART_ROOT}/docker"

echo "SCRIPT_DIR           :$SCRIPT_DIR"
echo "ZITI_QUICKSTART_ROOT :$ZITI_QUICKSTART_ROOT"
echo "ZITI_ROOT            :$ZITI_ROOT"
echo "ZITI_DOCKER_ROOT     :$ZITI_DOCKER_ROOT"

cd $ZITI_ROOT
rm -rf ./linux-build
mkdir linux-build
echo "building"
go build -o linux-build ./...
echo "built..."

if [ -d "${ZITI_DOCKER_ROOT}/image/ziti.ignore" ]; then
  rm -rf "${ZITI_DOCKER_ROOT}/image/ziti.ignore"
fi

mkdir "${ZITI_DOCKER_ROOT}/image/ziti.ignore"
cp "${ZITI_ROOT}/linux-build/"* "${ZITI_DOCKER_ROOT}/image/ziti.ignore"

cd "${ZITI_DOCKER_ROOT}"
docker build "${SCRIPT_DIR}/image" -t openziti/quickstart:dev

if [ -d "${ZITI_DOCKER_ROOT}/image/ziti.ignore" ]; then
  rm -rf "${ZITI_DOCKER_ROOT}/image/ziti.ignore"
fi

echo "Ziti Version: $(docker run --rm -it openziti/quickstart:dev /openziti/ziti-bin/ziti --version)"

vers="$(echo "${ZITI_BINARIES_VERSION}" | cut -c 2-100)"
docker tag "openziti/quickstart:dev" "openziti/quickstart:dev"

echo "to complete the quickstart:dev push, issue:"
echo ""
echo "docker push \"openziti/quickstart:dev\""

