#!/usr/bin/env bash

# exec this script with BASH v4+ on Linux to test the checked-out ziti repo's Docker controller and router deployments

set -o errexit
set -o nounset
set -o pipefail
set -o errtrace

KEEP=0
_exit_code=0
_in_err_handler=0
_err_handler() {
    _exit_code=$?
    if (( _in_err_handler )); then return; fi
    _in_err_handler=1
    echo "ERROR: FAILED at line ${LINENO}: ${BASH_COMMAND} (exit ${_exit_code})" >&2
    # Dump container logs for diagnostics
    docker compose logs --tail=200 >&2 2>/dev/null || true
}
trap '_err_handler' ERR

cleanup(){
    # Disable errexit in cleanup — every command is best-effort
    set +o errexit
    if (( KEEP )); then
        echo "DEBUG: keeping test instance (--keep)" >&2
        return 0
    fi
    if [[ -t 0 ]]; then
        echo "Removing these containers and their volumes: ziti-controller1, ziti-controller2, ziti-controller3, ziti-router1, ziti-router2, ziti-router3, ziti-test in 30s. Re-run with </dev/null to skip this delay." >&2
        sleep 30
    fi
	docker compose --profile test down --volumes --remove-orphans
    echo "DEBUG: cleanup complete"
}
trap 'cleanup; exit $_exit_code' EXIT

# Parse CLI flags
while [[ $# -gt 0 ]]; do
    case "$1" in
        --keep) KEEP=1; shift ;;
        *) break ;;
    esac
done

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

: "${ZIGGY_UID:=$(id -u)}"
: "${ZITI_GO_VERSION:=$(grep -E '^go [0-9]+\.[0-9]*' "./go.mod" | cut -d " " -f2)}"

COMPOSE_FILE_VALUE="${REPOROOT}/dist/docker-images/ziti-controller/compose.yml:${REPOROOT}/dist/docker-images/ziti-controller/compose.test.yml:${REPOROOT}/dist/docker-images/ziti-router/compose.yml:${REPOROOT}/dist/docker-images/ziti-router/compose.test.yml"

# With args: pass through to docker compose (e.g., ./docker.test.bash ps, logs -f)
# Without args: run the full test
if (( $# )); then
    export COMPOSE_FILE="${COMPOSE_FILE_VALUE}" ZIGGY_UID
    exec docker compose "$@"
fi

set -o xtrace

declare -a BINS=(grep docker go nc curl)
for BIN in "${BINS[@]}"; do
    checkCommand "$BIN"
done

ZITI_USER="admin"
ZITI_PWD="ziggypw"
ZITI_CTRL_ADVERTISED_ADDRESS="ziti-controller1.127.0.0.1.sslip.io"
ZITI_CTRL_ADVERTISED_PORT="1281"
ZITI_CLUSTER_NODE_NAME="ziti-controller1"
ZITI_CLUSTER_TRUST_DOMAIN="${ZITI_CTRL_ADVERTISED_ADDRESS#*.}"
ZITI_CTRL2_ADVERTISED_ADDRESS="ziti-controller2.127.0.0.1.sslip.io"
ZITI_CTRL2_ADVERTISED_PORT="1282"
ZITI_CTRL2_NODE_NAME="ziti-controller2"
ZITI_CTRL3_ADVERTISED_ADDRESS="ziti-controller3.127.0.0.1.sslip.io"
ZITI_CTRL3_ADVERTISED_PORT="1283"
ZITI_CTRL3_NODE_NAME="ziti-controller3"
ZITI_ROUTER_PORT="30222"
ZITI_RTR2_PORT="30223"
ZITI_RTR3_PORT="30224"
ZITI_CONTROLLER_IMAGE="ziti-controller:local"
ZITI_ROUTER_IMAGE="ziti-router:local"
ZITI_ROUTER_NAME="router1"
ZITI_RTR2_NAME="router2"
ZITI_RTR3_NAME="router3"

export COMPOSE_FILE="${COMPOSE_FILE_VALUE}" \
ZIGGY_UID \
ZITI_GO_VERSION \
ZITI_USER \
ZITI_PWD \
ZITI_CTRL_ADVERTISED_ADDRESS \
ZITI_CTRL_ADVERTISED_PORT \
ZITI_CLUSTER_NODE_NAME \
ZITI_CLUSTER_TRUST_DOMAIN \
ZITI_CTRL2_ADVERTISED_ADDRESS \
ZITI_CTRL2_ADVERTISED_PORT \
ZITI_CTRL2_NODE_NAME \
ZITI_CTRL3_ADVERTISED_ADDRESS \
ZITI_CTRL3_ADVERTISED_PORT \
ZITI_CTRL3_NODE_NAME \
ZITI_ROUTER_PORT \
ZITI_RTR2_PORT \
ZITI_RTR3_PORT \
ZITI_CONTROLLER_IMAGE \
ZITI_ROUTER_IMAGE \
ZITI_ROUTER_NAME \
ZITI_RTR2_NAME \
ZITI_RTR3_NAME

export ZITI_ROUTER_ADVERTISED_ADDRESS="${ZITI_ROUTER_NAME}.127.0.0.1.sslip.io" \
ZITI_ENROLL_TOKEN="/home/ziggy/.config/ziti/${ZITI_ROUTER_NAME}.jwt" \
ZITI_ROUTER_LISTENER_BIND_PORT="${ZITI_ROUTER_PORT}" \
ZITI_RTR2_ADVERTISED_ADDRESS="${ZITI_RTR2_NAME}.127.0.0.1.sslip.io" \
ZITI_RTR3_ADVERTISED_ADDRESS="${ZITI_RTR3_NAME}.127.0.0.1.sslip.io"

cleanup

for PORT in "${ZITI_CTRL_ADVERTISED_PORT}" "${ZITI_CTRL2_ADVERTISED_PORT}" "${ZITI_CTRL3_ADVERTISED_PORT}" "${ZITI_ROUTER_PORT}" "${ZITI_RTR2_PORT}" "${ZITI_RTR3_PORT}"
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

# entrypoint.bash now handles cluster initialization automatically when
# ZITI_BOOTSTRAP_CLUSTER=true and ZITI_PWD is set
docker compose up wait-for-controller

# helper: log in to the controller from inside the container
ctrl_login() {
    docker compose exec -T ziti-controller1 /bin/bash -euxc "
        ziti edge login \
            \${ZITI_CTRL_ADVERTISED_ADDRESS}:\${ZITI_CTRL_ADVERTISED_PORT} \
            --ca=/ziti-controller/pki/root/certs/root.cert \
            --username=\${ZITI_USER} \
            --password=\${ZITI_PWD} \
            --timeout=1 \
            --verbose
    "
}

# helper: create an edge router and write its JWT to the shared volume
create_router() {
    local _name="$1" _jwt_path="$2"
    docker compose exec -T ziti-controller1 /bin/bash -euxc "
        ziti edge create edge-router '${_name}' -to '${_jwt_path}'
    "
}

# helper: wait for a router to come online by name
wait_router_online() {
    local _name="$1"
    local _attempts=10 _delay=3
    until ! (( _attempts )) || [[ $(docker compose exec -T ziti-controller1 ziti edge list edge-routers -j "name=\"${_name}\"" | jq -r '.data[0].isOnline') == "true" ]]
    do
        (( _attempts-- ))
        echo "INFO: waiting for ${_name} to be online"
        sleep ${_delay}
    done
    if [[ $(docker compose exec -T ziti-controller1 ziti edge list edge-routers -j "name=\"${_name}\"" | jq -r '.data[0].isOnline') == "true" ]]
    then
        echo "INFO: ${_name} is online"
    else
        echo "ERROR: ${_name} is offline" >&2
        exit 1
    fi
}

# log in and create the first router
ATTEMPTS=10
DELAY=2
until ctrl_login; do
    if (( ATTEMPTS-- == 0 )); then
        echo "ERROR: failed to log in to controller" >&2
        exit 1
    fi
    sleep ${DELAY}
done

create_router "${ZITI_ROUTER_NAME}" "/home/ziggy/.config/ziti/${ZITI_ROUTER_NAME}.jwt"

# start router1 alongside controller1
docker compose up ziti-router1 --detach
wait_router_online "${ZITI_ROUTER_NAME}"

# join controller2, then start router2
ATTEMPTS=20
DELAY=3
until docker compose exec -T ziti-controller2 ziti agent cluster add \
        "tls:${ZITI_CTRL_ADVERTISED_ADDRESS}:${ZITI_CTRL_ADVERTISED_PORT}"; do
    if (( ATTEMPTS-- == 0 )); then
        echo "ERROR: ziti-controller2 failed to join the cluster" >&2
        exit 1
    fi
    echo "INFO: retrying cluster add for ziti-controller2 (${ATTEMPTS} attempts left)"
    sleep ${DELAY}
done
echo "INFO: ziti-controller2 joined the cluster"

create_router "${ZITI_RTR2_NAME}" "/home/ziggy/.config/ziti/${ZITI_RTR2_NAME}.jwt"
# read the JWT from the shared volume and pass it to router2
ZITI_RTR2_ENROLL_TOKEN="$(docker compose exec -T ziti-controller1 cat "/home/ziggy/.config/ziti/${ZITI_RTR2_NAME}.jwt")"
export ZITI_RTR2_ENROLL_TOKEN
docker compose up ziti-router2 --detach
wait_router_online "${ZITI_RTR2_NAME}"

# join controller3, then start router3
ATTEMPTS=20
DELAY=3
until docker compose exec -T ziti-controller3 ziti agent cluster add \
        "tls:${ZITI_CTRL_ADVERTISED_ADDRESS}:${ZITI_CTRL_ADVERTISED_PORT}"; do
    if (( ATTEMPTS-- == 0 )); then
        echo "ERROR: ziti-controller3 failed to join the cluster" >&2
        exit 1
    fi
    echo "INFO: retrying cluster add for ziti-controller3 (${ATTEMPTS} attempts left)"
    sleep ${DELAY}
done
echo "INFO: ziti-controller3 joined the cluster"

create_router "${ZITI_RTR3_NAME}" "/home/ziggy/.config/ziti/${ZITI_RTR3_NAME}.jwt"
ZITI_RTR3_ENROLL_TOKEN="$(docker compose exec -T ziti-controller1 cat "/home/ziggy/.config/ziti/${ZITI_RTR3_NAME}.jwt")"
export ZITI_RTR3_ENROLL_TOKEN
docker compose up ziti-router3 --detach
wait_router_online "${ZITI_RTR3_NAME}"

# verify the cluster has 3 members
CLUSTER_OUTPUT="$(docker compose exec -T ziti-controller1 ziti agent cluster list 2>/dev/null)"
CLUSTER_SIZE="$(echo "${CLUSTER_OUTPUT}" | grep -c 'tls:')" || true
if (( CLUSTER_SIZE < 3 )); then
    echo "ERROR: expected 3 cluster members, found ${CLUSTER_SIZE}" >&2
    echo "${CLUSTER_OUTPUT}" >&2
    exit 1
fi
echo "INFO: cluster has ${CLUSTER_SIZE} members"

# verify all 3 routers are online
ROUTER_COUNT="$(docker compose exec -T ziti-controller1 ziti edge list edge-routers -j | jq '[.data[] | select(.isOnline == true)] | length')"
if (( ROUTER_COUNT < 3 )); then
    echo "ERROR: expected 3 online routers, found ${ROUTER_COUNT}" >&2
    docker compose exec -T ziti-controller1 ziti edge list edge-routers
    exit 1
fi
echo "INFO: all ${ROUTER_COUNT} routers are online"

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
until ! (( ATTEMPTS-- )) || eval "${curl_cmd}" &> /dev/null
do
    echo "Waiting for zac"
    sleep ${DELAY}
done
eval "${curl_cmd}"

# cleanup runs via EXIT trap
