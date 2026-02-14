#!/usr/bin/env bash

# exec this script with BASH v4+ on Linux to test the checked-out ziti repo's Kubernetes controller and router deployments

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

cleanup(){
    if ! (( I_AM_ROBOT ))
    then
        echo "WARNING: destroying minikube profile ${ZITI_NAMESPACE} in 30s; set I_AM_ROBOT=1 to suppress this message" >&2
        sleep 30
    fi
	if minikube --profile "${ZITI_NAMESPACE}" delete
    then
        echo "DEBUG: cleanup complete"
    else
        echo "WARNING: error during cleanup"
    fi
    return 0
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

declare -a BINS=(grep go nc docker minikube ./quickstart/kubernetes/miniziti.bash)
for BIN in "${BINS[@]}"; do
    checkCommand "$BIN"
done


: "${I_AM_ROBOT:=0}"
: "${ZITI_GO_VERSION:=$(grep -E '^go \d+\.\d*' "./go.mod" | cut -d " " -f2)}"
: "${ZITI_NAMESPACE:="zititest"}"

declare -a MINIKUBE_START_ARGS=()
declare -a MINIZITI_START_ARGS=()
while (( "$#" )); do
    case "$1" in
        --charts)
            [[ "$#" -ge 2 ]] || {
                echo "ERROR: --charts requires a value" >&2
                exit 1
            }
            MINIZITI_START_ARGS+=("--charts" "$2")
            shift 2
        ;;
        --charts=*)
            MINIZITI_START_ARGS+=("--charts" "${1#*=}")
            shift
        ;;
        *)
            MINIKUBE_START_ARGS+=("$1")
            shift
        ;;
    esac
done

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
minikube --profile "${ZITI_NAMESPACE}" start "${MINIKUBE_START_ARGS[@]}"
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

bash -x ./quickstart/kubernetes/miniziti.bash start \
--profile "${ZITI_NAMESPACE}" \
--no-hosts --devel \
--values-dir "${EXTRA_VALUES_DIR}" \
"${MINIZITI_START_ARGS[@]}"

ZITI_INGRESS_ZONE=$(
    minikube kubectl --profile "${ZITI_NAMESPACE}" -- \
        --context "${ZITI_NAMESPACE}" \
        get configmap "miniziti-config" \
        --namespace "${ZITI_NAMESPACE}" \
        -o jsonpath='{.data.ingress-zone}' 2>/dev/null || true
)
if [[ -z "${ZITI_INGRESS_ZONE}" ]]; then
    TRAEFIK_LB_IP=$(
        minikube kubectl --profile "${ZITI_NAMESPACE}" -- \
            --context "${ZITI_NAMESPACE}" \
            get service traefik -n traefik -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || true
    )
    if [[ -n "${TRAEFIK_LB_IP}" ]]; then
        ZITI_INGRESS_ZONE="${TRAEFIK_LB_IP}.sslip.io"
    else
        echo "ERROR: failed to determine ingress zone from configmap or Traefik LoadBalancer IP" >&2
        exit 1
    fi
fi
ZITI_CTRL_ADVERTISED_ADDRESS="miniziti-controller.${ZITI_INGRESS_ZONE}"

# verify console is available
curl -skSfw '%{http_code}\t%{url}\n' -o/dev/null "https://${ZITI_CTRL_ADVERTISED_ADDRESS}:${ZITI_CTRL_ADVERTISED_PORT}/zac/"

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
ZITI_CTRL_EDGE_ADVERTISED_ADDRESS="${ZITI_CTRL_ADVERTISED_ADDRESS}" \
ZITI_CTRL_EDGE_ADVERTISED_PORT="${ZITI_CTRL_ADVERTISED_PORT}" \
ZITI_TEST_BIND_ADDRESS="ziti-controller-client.${ZITI_NAMESPACE}.svc.cluster.local"

_test_result=$(go test -v -count=1 -tags="quickstart manual" ./ziti/run/...)

# check for failure modes that don't result in an error exit code
if [[ "${_test_result}" =~ "no tests to run" ]]
then
    echo "ERROR: test failed because no tests to run"
    exit 1
fi

cleanup
