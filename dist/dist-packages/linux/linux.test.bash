#!/usr/bin/env bash

# exec this script with BASH v4+ on Linux to test the checked-out ziti repo's Linux controller and router deployments

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

cleanup(){
    if ! (( I_AM_ROBOT ))
    then
        echo "WARNING: destroying all controller and router state files in 30s; set I_AM_ROBOT=1 to suppress this message" >&2
        sleep 30
    fi
    for SVC in ziti-{router,controller}.service
    do
    (set +e
        sudo systemctl stop "${SVC}"
        sudo systemctl disable --now "${SVC}"
        sudo systemctl reset-failed "${SVC}"
        sudo systemctl clean --what=state "${SVC}"
    )||true
    done
    for ETC in router controller
    do
        (set +e
            sudo apt-get remove --yes --purge "openziti-${ETC}"
            if [[ -d /opt/openziti/etc/${ETC} ]]
            then
                sudo rm -r "/opt/openziti/etc/${ETC}"
            fi
        )||true
        if [[ -s /opt/openziti/etc/${ETC}/bootstrap.env ]]
        then
            rm -f /opt/openziti/etc/${ETC}/bootstrap.env
        fi
    done
    if [[ -d "${ZITI_CONSOLE_LOCATION}" ]]
    then
        sudo rm -rf "${ZITI_CONSOLE_LOCATION}"
    fi
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
REPOROOT="$(cd "${BASEDIR}/../../.." && pwd)"
cd "${REPOROOT}"

declare -a BINS=(grep go nc docker curl unzip)
for BIN in "${BINS[@]}"; do
    checkCommand "$BIN"
done

: "${I_AM_ROBOT:=0}"
: "${ZITI_GO_VERSION:=$(grep -E '^go \d+\.\d*' "./go.mod" | cut -d " " -f2)}"
: "${ZITI_USER:=admin}"
: "${ZITI_PWD:=ziggypw}"
: "${TMPDIR:=$(mktemp -d)}"
: "${ZITI_CTRL_ADVERTISED_ADDRESS:="ctrl1.127.0.0.1.sslip.io"}"
: "${ZITI_CTRL_ADVERTISED_PORT:="12801"}"
: "${ZITI_ROUTER_PORT:="30223"}"
: "${ZITI_ROUTER_NAME:="router1"}"
: "${ZITI_ROUTER_ADVERTISED_ADDRESS:="${ZITI_ROUTER_NAME}.127.0.0.1.sslip.io"}"
: "${ZITI_ENROLL_TOKEN:="${TMPDIR}/${ZITI_ROUTER_NAME}.jwt"}"
: "${ZITI_CONSOLE_LOCATION:="/opt/openziti/share/consoletest"}"
# default is amd64 image manifest matching .github/actions/nfpm/action.yml
: "${NFPM_VERSION:=docker.io/goreleaser/nfpm:v2.42.0@sha256:31c856f5806306ba105111fec19e8679222c67c43ad09921a46f9400d99bbbb1}"

export \
ZITI_GO_VERSION \
ZITI_USER \
ZITI_PWD \
ZITI_CTRL_ADVERTISED_ADDRESS \
ZITI_CTRL_ADVERTISED_PORT \
ZITI_CONSOLE_LOCATION \
ZITI_ROUTER_PORT \
ZITI_ROUTER_NAME \
ZITI_ROUTER_ADVERTISED_ADDRESS \
ZITI_ENROLL_TOKEN \
DEBUG=1 \

cleanup

for PORT in "${ZITI_CTRL_ADVERTISED_PORT}" "${ZITI_ROUTER_PORT}"
do
    portcheck "${PORT}"
done

# build ziti in the ./release dir where nfpm will look for it to build the package
mkdir -p ./release
go build -o ./release/ ./...

docker_nfpm(){
    docker run --rm \
    --user "${UID:-0}" \
    --volume "${TMPDIR}":/mnt/packages \
    --volume ./dist:/mnt/dist \
    --volume ./release:/mnt/release \
    --workdir /mnt \
    --env ZITI_VENDOR \
    --env ZITI_MAINTAINER \
    --env ZITI_HOMEPAGE \
    --env MINIMUM_SYSTEMD_VERSION \
    "${NFPM_VERSION}" \
    "${@}"
}

for PKG in openziti{,-controller,-router}
do
    ZITI_HOMEPAGE="https://openziti.io" \
    ZITI_VENDOR="netfoundry" \
    ZITI_MAINTAINER="Maintainers <developers@openziti.org>" \
    MINIMUM_SYSTEMD_VERSION="232" \
    docker_nfpm pkg \
    --packager deb \
    --target  ./packages \
    --config "./dist/dist-packages/linux/nfpm-${PKG}.yaml"
done

sudo dpkg --install "${TMPDIR}/openziti_"*.deb
sudo dpkg --install "${TMPDIR}/openziti-"{controller,router}_*.deb

sudo -E bash -x /opt/openziti/etc/controller/bootstrap.bash

sudo systemctl start ziti-controller.service
sudo systemd-run \
--wait --quiet \
--service-type=oneshot \
--property=TimeoutStartSec=20s \
systemctl is-active ziti-controller.service

# shellcheck disable=SC2140
zitiLogin(){
    ziti edge login "${ZITI_CTRL_ADVERTISED_ADDRESS}:${ZITI_CTRL_ADVERTISED_PORT}" \
    --yes \
    --username "${ZITI_USER}" \
    --password "${ZITI_PWD}"
}
ATTEMPTS=9
DELAY=3
until ! (( --ATTEMPTS )) || zitiLogin
do
    echo "Waiting for controller login"
    sleep ${DELAY}
done
if ! (( ATTEMPTS )); then
    echo "ERROR: timeout waiting for controller login" >&2
    exit 1
fi

zitiRouter() {
    ziti edge create edge-router "${ZITI_ROUTER_NAME}" -to "${ZITI_ENROLL_TOKEN}"
}

ATTEMPTS=9
DELAY=3
until ! (( --ATTEMPTS )) || zitiRouter
do
    echo "Waiting for router creation"
    sleep ${DELAY}

done
if ! (( ATTEMPTS )); then
    echo "ERROR: timeout waiting for router creation" >&2
    exit 1
fi

# mock ziti console html
sudo mkdir -p "${ZITI_CONSOLE_LOCATION}"
sudo tee "${ZITI_CONSOLE_LOCATION}/index.html" <<< "I am ZAC"
sudo chmod -R +rX "${ZITI_CONSOLE_LOCATION}"

sudo -E bash -x /opt/openziti/etc/router/bootstrap.bash

sudo systemctl start ziti-router.service
sudo systemd-run \
--wait --quiet \
--service-type=oneshot \
--property=TimeoutStartSec=20s \
systemctl is-active ziti-router.service

isOnline(){
    ziti edge list edge-routers -j | jq '.data[0].isOnline'
}
ATTEMPTS=9
DELAY=3
until ! (( --ATTEMPTS )) || [[ "$(isOnline)" == "true" ]]
do
    echo "INFO: waiting for router to be online"
    sleep ${DELAY}
done
if [[ "$(isOnline)" == "true" ]]
then
    echo "INFO: router is online"
else
    echo "INFO: router is offline"
    exit 1
fi

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
getZac(){
    curl -kfw '%{http_code}\t%{url}\n' -o/dev/null \
        "https://${ZITI_CTRL_ADVERTISED_ADDRESS}:${ZITI_CTRL_ADVERTISED_PORT}/zac/"
}

until ! (( --ATTEMPTS )) || getZac &> /dev/null
do
    echo "Waiting for zac"
    sleep ${DELAY}
done
if ! (( ATTEMPTS )); then
    echo "ERROR: timeout waiting for zac" >&2
    exit 1
fi

cleanup
