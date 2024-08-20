#!/usr/bin/env bash

# exec this script from the root directory of the repository to test the Docker deployment

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

cleanup(){
	docker compose --profile test down --volumes --remove-orphans
}

[[ -s ./go.work ]] && {
	echo "ERROR: remove go.work before testing deployment" >&2
	exit 1
}

: "${ZIGGY_UID:=$(id -u)}"
: "${ZITI_GO_VERSION:=$(grep -Po '^go\s+\K\d+\.\d+(\.\d+)?$' ./go.mod)}"

export COMPOSE_FILE=\
./dist/docker-images/ziti-controller/compose.yml\
:./dist/docker-images/ziti-controller/compose.test.yml\
:./dist/docker-images/ziti-router/compose.yml\
:./dist/docker-images/ziti-router/compose.test.yml \
ZIGGY_UID \
ZITI_GO_VERSION \
ZITI_PWD="ziggypw" \
ZITI_CTRL_ADVERTISED_ADDRESS="ctrl1.127.21.71.0.sslip.io" \
ZITI_CONTROLLER_IMAGE="ziti-controller:local" \
ZITI_ROUTER_IMAGE="ziti-router:local" \
ZITI_ROUTER_NAME="router1"

export ZITI_ROUTER_ADVERTISED_ADDRESS="${ZITI_ROUTER_NAME}.127.21.71.0.sslip.io" \
ZITI_ENROLL_TOKEN="/home/ziggy/.config/ziti/${ZITI_ROUTER_NAME}.jwt"

mkdir -p ./release/amd64/linux
go build -o ./release/amd64/linux ./...

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

cleanup

docker compose up ziti-login

docker compose run --rm --entrypoint=/bin/bash --env ZITI_ROUTER_NAME="${ZITI_ROUTER_NAME}" ziti-login \
-euxc 'ziti edge create edge-router "${ZITI_ROUTER_NAME}" -to ~ziggy/.config/ziti/"${ZITI_ROUTER_NAME}.jwt"'

docker compose up ziti-router --detach

docker compose run --rm quickstart-test

cleanup
