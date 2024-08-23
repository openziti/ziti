#!/usr/bin/env bash

# exec this script with BASH v4+ on Linux to test the checked-out ziti repo's Kubernetes controller and router deployments

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

cleanup(){
	bash ./quickstart/kubernetes/miniziti.bash delete --profile "${ZITI_NAMESPACE}" ${I_AM_ROBOT:+--now}
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

BASEDIR="$(cd "$(dirname "${0}")" && pwd)"
REPOROOT="$(cd "${BASEDIR}/../.." && pwd)"
cd "${REPOROOT}"

: "${ZITI_GO_VERSION:=$(grep -E '^go \d+\.\d*' "./go.mod" | cut -d " " -f2)}"
: "${ZITI_NAMESPACE:="zititest"}"

cleanup

arch="$(go env GOARCH)" 
mkdir -p "./release/$arch/linux"
go build -o "./release/$arch/linux" ./...

ZITI_CLI_IMAGE="ziti-cli"
ZITI_CLI_TAG="local"
ZITI_CONTROLLER_IMAGE="ziti-controller:local"
ZITI_ROUTER_IMAGE="ziti-router:local"

# eval "$(minikube --profile "${ZITI_NAMESPACE}" docker-env)"

# build from cache on Docker host 
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

export \
ZITI_CTRL_ADVERTISED_PORT="443" \
ZITI_ROUTER_PORT="443"

for PORT in "${ZITI_CTRL_ADVERTISED_PORT}" "${ZITI_ROUTER_PORT}"
do
	portcheck "${PORT}"
done

# load container images in minikube
minikube --profile "${ZITI_NAMESPACE}" start "${@}"
for IMG in "${ZITI_CONTROLLER_IMAGE}" "${ZITI_ROUTER_IMAGE}"
do
    minikube --profile "${ZITI_NAMESPACE}" image load "${IMG}"
done

# use the locally built controller and router images in minikube
EXTRA_VALUES_DIR=$(mktemp -d)
cat << CTRL > "${EXTRA_VALUES_DIR}/ziti-controller.yaml"
image:
    repository: ${ZITI_CONTROLLER_IMAGE%:*}
    tag: ${ZITI_CONTROLLER_IMAGE#*:}
    pullPolicy: Never
CTRL
cat << ROUTER > "${EXTRA_VALUES_DIR}/ziti-router.yaml"
image:
    repository: ${ZITI_ROUTER_IMAGE%:*}
    tag: ${ZITI_ROUTER_IMAGE#*:}
    pullPolicy: Never
ROUTER

./quickstart/kubernetes/miniziti.bash start \
--profile "${ZITI_NAMESPACE}" \
--no-hosts \
--values-dir "${EXTRA_VALUES_DIR}"

MINIKUBE_IP="$(minikube --profile "${ZITI_NAMESPACE}" ip)"

# verify console is available
curl -skSfw '%{http_code}\t%{url}\n' -o/dev/null "https://miniziti-controller.${MINIKUBE_IP}.sslip.io/zac/"

ZITI_PWD=$(
minikube kubectl --profile "${ZITI_NAMESPACE}" -- \
--context "${ZITI_NAMESPACE}" \
get secrets "ziti-controller-admin-secret" \
--namespace "${ZITI_NAMESPACE}" \
--output go-template='{{index .data "admin-password" | base64decode }}'
)


export \
ZITI_PWD \
ZITI_ROUTER_NAME="miniziti-router" \
ZITI_CTRL_ADVERTISED_ADDRESS="miniziti-controller.${MINIKUBE_IP}.sslip.io"

# ZITI_CTRL_EDGE_ADVERTISED_ADDRESS=${ZITI_CTRL_ADVERTISED_ADDRESS} \
# ZITI_CTRL_EDGE_ADVERTISED_PORT=${ZITI_CTRL_ADVERTISED_PORT} \

ZITI_TEST_BIND_ADDRESS="ziti-controller-client.zititest.svc.cluster.local" \
go test -v -count=1 -tags="quickstart manual" ./ziti/cmd/edge/...

cleanup
