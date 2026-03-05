#!/usr/bin/env bash

# exec this script with BASH v4+ on Linux to test the checked-out ziti repo's Kubernetes controller and router deployments

set -o errexit
set -o nounset
set -o pipefail
set -o errtrace
set -o xtrace

NO_DESTROY=0
_exit_code=0
_in_err_handler=0
_err_handler() {
    _exit_code=$?
    if (( _in_err_handler )); then return; fi
    _in_err_handler=1
    echo "ERROR: FAILED at line ${LINENO}: ${BASH_COMMAND} (exit ${_exit_code})" >&2
    # Dump pod status and logs for diagnostics
    (
        set +e
        minikube kubectl --profile "${ZITI_NAMESPACE:-zititest}" -- \
            --context "${ZITI_NAMESPACE:-zititest}" \
            get pods -A 2>&1 || true
        for _ns in "${ZITI_NAMESPACE:-zititest}" traefik; do
            minikube kubectl --profile "${ZITI_NAMESPACE:-zititest}" -- \
                --context "${ZITI_NAMESPACE:-zititest}" \
                logs -n "${_ns}" --all-containers --tail=100 2>&1 || true
        done
    ) >&2
}
trap '_err_handler' ERR

cleanup(){
    # Disable errexit in cleanup — every command is best-effort
    set +o errexit
    if (( NO_DESTROY )); then
        echo "DEBUG: cleanup skipped (--no-destroy)" >&2
        return 0
    fi
    if [[ -t 0 ]]; then
        echo "Deleting minikube profile '${ZITI_NAMESPACE}' in 30s. Re-run with </dev/null to skip this delay." >&2
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
trap 'cleanup; exit $_exit_code' EXIT

# Parse CLI flags
while [[ $# -gt 0 ]]; do
    case "$1" in
        --no-destroy) NO_DESTROY=1; shift ;;
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

declare -a BINS=(grep go nc docker minikube ./quickstart/kubernetes/miniziti.bash)
for BIN in "${BINS[@]}"; do
    checkCommand "$BIN"
done


: "${ZITI_GO_VERSION:=$(grep -E '^go [0-9]+\.[0-9]*' "./go.mod" | cut -d " " -f2)}"
: "${ZITI_NAMESPACE:="zititest"}"

# With a miniziti subcommand: pass through (e.g., ./k8s.test.bash kubectl get pods)
# Without args or with flags: run the full test
case "${1:-}" in
    kubectl|minikube|shell|ziti|creds|console|status|login|delete)
        exec ./quickstart/kubernetes/miniziti.bash --profile "${ZITI_NAMESPACE}" "$@"
    ;;
esac

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
ZITI_CTRL_ADVERTISED_ADDRESS="${ZITI_NAMESPACE}-controller.${ZITI_INGRESS_ZONE}"

# verify console is available (retry — the endpoint may need a moment after deployment)
for (( _attempt = 1; _attempt <= 10; _attempt++ )); do
    _status=$(curl -skSfw '%{http_code}' -o/dev/null "https://${ZITI_CTRL_ADVERTISED_ADDRESS}:${ZITI_CTRL_ADVERTISED_PORT}/zac/" 2>/dev/null || true)
    echo "attempt ${_attempt}/10: HTTP ${_status} — https://${ZITI_CTRL_ADVERTISED_ADDRESS}:${ZITI_CTRL_ADVERTISED_PORT}/zac/"
    [[ "${_status}" == "200" ]] && break
    if (( _attempt == 10 )); then
        echo "ERROR: ZAC console returned HTTP ${_status} after 10 attempts" >&2
        exit 1
    fi
    sleep 6
done

ZITI_PWD=$(
    minikube kubectl --profile "${ZITI_NAMESPACE}" -- \
        --context "${ZITI_NAMESPACE}" \
        get secrets "ziti-controller1-admin-secret" \
        --namespace "${ZITI_NAMESPACE}" \
        --output go-template='{{index .data "admin-password" | base64decode }}'
)


export \
ZITI_PWD \
ZITI_ROUTER_NAME="${ZITI_NAMESPACE}-router" \
ZITI_CTRL_EDGE_ADVERTISED_ADDRESS="${ZITI_CTRL_ADVERTISED_ADDRESS}" \
ZITI_CTRL_EDGE_ADVERTISED_PORT="${ZITI_CTRL_ADVERTISED_PORT}" \
ZITI_TEST_BIND_ADDRESS="ziti-controller1-client.${ZITI_NAMESPACE}.svc.cluster.local"

_test_result=$(go test -v -count=1 -tags="quickstart manual" ./ziti/run/...)

# check for failure modes that don't result in an error exit code
if [[ "${_test_result}" =~ "no tests to run" ]]
then
    echo "ERROR: test failed because no tests to run"
    exit 1
fi

# cleanup runs via EXIT trap
