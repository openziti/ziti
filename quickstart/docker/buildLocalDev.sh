#!/usr/bin/env bash
set -eo pipefail

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
ZITI_QUICKSTART_ROOT="$(realpath ${SCRIPT_DIR}/..)"
ZITI_ROOT="$(realpath ${ZITI_QUICKSTART_ROOT}/..)"
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

docker run --rm -it openziti/quickstart:dev /openziti/ziti-bin/ziti version

vers="$(echo "${ZITI_BINARIES_VERSION}" | cut -c 2-100)"
docker tag "openziti/quickstart:dev" "openziti/quickstart:dev"

echo "to complete the quickstart:dev push, issue:"
echo ""
echo "docker push \"openziti/quickstart:dev\""

