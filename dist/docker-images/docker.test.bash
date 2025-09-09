#!/usr/bin/env bash

# exec this script with BASH v4+ on Linux to test the checked-out ziti repo's Docker controller and router deployments

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

cleanup(){
    if ! (( I_AM_ROBOT ))
    then
        echo "WARNING: destroying all controller and router state volumes in 30s; set I_AM_ROBOT=1 to suppress this message" >&2
        sleep 30
    fi
	docker compose --profile test down --volumes --remove-orphans
    echo "DEBUG: cleanup complete"
}

portcheck(){
    PORT="${1}"
    if nc -zv localhost "$PORT" &>/dev/null
    then
		echo "ERROR: port $PORT is already allocated" >&2
        return 1
    else
		echo "DEBUG: port $PORT is available"
        return 0
    fi
}

checkCommand() {
    if ! command -v "$1" &>/dev/null; then
        logError "this script requires command '$1'. Please install on the search PATH and try again."
        $1
    fi
}

BASEDIR="$(cd "$(dirname "${0}")" && pwd)"
REPOROOT="$(cd "${BASEDIR}/../.." && pwd)"
cd "${REPOROOT}"

declare -a BINS=(grep docker go nc curl)
for BIN in "${BINS[@]}"; do
    checkCommand "$BIN"
done

: "${I_AM_ROBOT:=0}"
: "${ZIGGY_UID:=$(id -u)}"
: "${ZITI_GO_VERSION:=$(grep -E '^go \d+\.\d*' "./go.mod" | cut -d " " -f2)}"

export COMPOSE_FILE=\
./dist/docker-images/ziti-controller/compose.yml\
:./dist/docker-images/ziti-controller/compose.test.yml\
:./dist/docker-images/ziti-router/compose.yml\
:./dist/docker-images/ziti-router/compose.test.yml \

export \
ZIGGY_UID \
ZITI_GO_VERSION \
ZITI_USER="admin" \
ZITI_PWD="ziggypw" \
ZITI_CLUSTER_TRUST_DOMAIN="127.0.0.1.sslip.io" \
ZITI_CLUSTER_NODE_NAME="ctrl1"

export \
ZITI_CTRL_ADVERTISED_ADDRESS="${ZITI_CLUSTER_NODE_NAME}.${ZITI_CLUSTER_TRUST_DOMAIN}" \
ZITI_CTRL_ADVERTISED_PORT="12800" \
ZITI_ROUTER_PORT="30222" \
ZITI_CONTROLLER_IMAGE="ziti-controller:local" \
ZITI_ROUTER_IMAGE="ziti-router:local" \
ZITI_ROUTER_NAME="router1"

export ZITI_ROUTER_ADVERTISED_ADDRESS="${ZITI_ROUTER_NAME}.127.0.0.1.sslip.io" \
ZITI_ENROLL_TOKEN="/home/ziggy/.config/ziti/${ZITI_ROUTER_NAME}.jwt" \
ZITI_ROUTER_LISTENER_BIND_PORT="${ZITI_ROUTER_PORT}"

cleanup

for PORT in "${ZITI_CTRL_ADVERTISED_PORT}" "${ZITI_ROUTER_PORT}"
do
	portcheck "${PORT}"
done

export GOOS="linux"
os="$(go env GOOS)"
arch="$(go env GOARCH)" 
mkdir -p "./release/$arch/$os"
go build -o "./release/$arch/$os" ./...

ZITI_CLI_IMAGE="ziti-cli"
ZITI_CLI_TAG="local"

docker build \
--build-arg "DOCKER_BUILD_DIR=./dist/docker-images/ziti-cli" \
--tag "${ZITI_CLI_IMAGE}:${ZITI_CLI_TAG}" \
--file "./dist/docker-images/ziti-cli/Dockerfile" \
"${PWD}"

docker build \
--build-arg "DOCKER_BUILD_DIR=./dist/docker-images/ziti-controller" \
--build-arg "ZITI_CLI_IMAGE=${ZITI_CLI_IMAGE}" \
--build-arg "ZITI_CLI_TAG=${ZITI_CLI_TAG}" \
--tag "${ZITI_CONTROLLER_IMAGE}" \
--file "./dist/docker-images/ziti-controller/Dockerfile" \
"${PWD}"

docker build \
--build-arg "DOCKER_BUILD_DIR=./dist/docker-images/ziti-router" \
--build-arg "ZITI_CLI_IMAGE=${ZITI_CLI_IMAGE}" \
--build-arg "ZITI_CLI_TAG=${ZITI_CLI_TAG}" \
--tag "${ZITI_ROUTER_IMAGE}" \
--file "./dist/docker-images/ziti-router/Dockerfile" \
"${PWD}"

DEBUG=1 docker compose up wait-for-controller

zitiLogin(){

    docker compose exec --no-TTY ziti-controller bash <<BASH

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

ziti edge login ${ZITI_CTRL_ADVERTISED_ADDRESS}:${ZITI_CTRL_ADVERTISED_PORT} \
--ca=/ziti-controller/pki/root/certs/root.cert \
--username=${ZITI_USER} \
--password=${ZITI_PWD} \
--timeout=1 \
--verbose;

BASH

}

ATTEMPTS=9
DELAY=3
until ! (( --ATTEMPTS )) || zitiLogin
do
    echo "DEBUG: ${ATTEMPTS} remaining attempts to login"
    sleep ${DELAY}
done
if ! (( ATTEMPTS ))
then
    echo "ERROR: ziti login failed" >&2
    exit 1
fi

zitiRouter() {
    docker compose exec --no-TTY ziti-controller bash <<BASH

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

ziti edge create edge-router "${ZITI_ROUTER_NAME}" -to ~ziggy/.config/ziti/"${ZITI_ROUTER_NAME}.jwt";

BASH
}

ATTEMPTS=9
DELAY=3

until ! (( --ATTEMPTS )) || zitiRouter
do
    echo "DEBUG: ${ATTEMPTS} remaining attempts to create router"
    sleep ${DELAY}
done
if ! (( ATTEMPTS ))
then
    echo "ERROR: ziti router creation failed" >&2
    exit 1
fi

DEBUG=1 docker compose up ziti-router --detach

unset GOOS
export \
ZITI_CTRL_EDGE_ADVERTISED_ADDRESS=${ZITI_CTRL_ADVERTISED_ADDRESS} \
ZITI_CTRL_EDGE_ADVERTISED_PORT=${ZITI_CTRL_ADVERTISED_PORT}

_test_result=$(go test -v -count=1 -tags="quickstart manual" ./ziti/run/...)

# check for failure modes that don't result in an error exit code
if [[ "${_test_result}" =~ "no tests to run" ]]
then
    echo "ERROR: test failed because no tests to run"
    exit 1
fi

ATTEMPTS=5
DELAY=3

# verify console is available
curl_cmd="curl -skSfw '%{http_code}\t%{url}\n' -o/dev/null \"https://${ZITI_CTRL_ADVERTISED_ADDRESS}:${ZITI_CTRL_ADVERTISED_PORT}/zac/\""
until ! (( --ATTEMPTS )) || eval "${curl_cmd}" &> /dev/null
do
    echo "DEBUG: ${ATTEMPTS} remaining attempts to verify zac"
    sleep ${DELAY}
done
eval "${curl_cmd}"

cleanup
