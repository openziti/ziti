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
    done
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
REPOROOT="$(cd "${BASEDIR}/../../.." && pwd)"
cd "${REPOROOT}"

: "${I_AM_ROBOT:=0}"
: "${ZITI_GO_VERSION:=$(grep -E '^go \d+\.\d*' "./go.mod" | cut -d " " -f2)}"
: "${ZITI_PWD:=ziggypw}"
: "${TMPDIR:=$(mktemp -d)}"
: "${ZITI_CTRL_ADVERTISED_ADDRESS:="ctrl1.127.0.0.1.sslip.io"}"
: "${ZITI_CTRL_ADVERTISED_PORT:="12801"}"
: "${ZITI_ROUTER_PORT:="30223"}"
: "${ZITI_ROUTER_NAME:="linux-router1"}"
: "${ZITI_ROUTER_ADVERTISED_ADDRESS:="${ZITI_ROUTER_NAME}.127.0.0.1.sslip.io"}"
: "${ZITI_ENROLL_TOKEN:="${TMPDIR}/${ZITI_ROUTER_NAME}.jwt"}"

export \
ZITI_GO_VERSION \
ZITI_PWD \
ZITI_CTRL_ADVERTISED_ADDRESS \
ZITI_CTRL_ADVERTISED_PORT \
ZITI_ROUTER_PORT \
ZITI_ROUTER_NAME \
ZITI_ROUTER_ADVERTISED_ADDRESS \
ZITI_ENROLL_TOKEN

cleanup

for PORT in "${ZITI_CTRL_ADVERTISED_PORT}" "${ZITI_ROUTER_PORT}"
do
    portcheck "${PORT}"
done

# build ziti in the ./release dir where nfpm will look for it to build the package
mkdir -p ./release
go build -o ./release/ ./...

for PKG in openziti{,-controller,-router}
do
    ZITI_HOMEPAGE="https://openziti.io" \
    ZITI_VENDOR="netfoundry" \
    ZITI_MAINTAINER="Maintainers <developers@openziti.org>" \
    MINIMUM_SYSTEMD_VERSION="232" \
    nfpm pkg \
    --packager deb \
    --target  "$TMPDIR" \
    --config "./dist/dist-packages/linux/nfpm-${PKG}.yaml"
done

sudo dpkg --install "${TMPDIR}/openziti_"*.deb
sudo dpkg --install "${TMPDIR}/openziti-"{controller,router}_*.deb

DEBUG=1 \
ZITI_ENROLL_TOKEN=/tmp/${ZITI_ROUTER_NAME}.jwt \
sudo /opt/openziti/etc/controller/bootstrap.bash << CTRL
ZITI_CTRL_ADVERTISED_ADDRESS="${ZITI_CTRL_ADVERTISED_ADDRESS}"
ZITI_CTRL_ADVERTISED_PORT="${ZITI_CTRL_ADVERTISED_PORT}"
ZITI_USER="admin"
ZITI_PWD="${ZITI_PWD}"
CTRL

sudo systemctl start ziti-controller.service
sudo systemd-run \
--wait --quiet \
--service-type=oneshot \
--property=TimeoutStartSec=20s \
systemctl is-active ziti-controller.service

# shellcheck disable=SC2140
login_cmd="ziti edge login ${ZITI_CTRL_ADVERTISED_ADDRESS}:${ZITI_CTRL_ADVERTISED_PORT}"\
" --yes"\
" --username admin"\
" --password ${ZITI_PWD}"
ATTEMPTS=10
DELAY=3
until ! ((ATTEMPTS)) || ${login_cmd}
do
    (( ATTEMPTS-- ))
    echo "Waiting for controller login"
    sleep ${DELAY}
done
ziti edge create edge-router "${ZITI_ROUTER_NAME}" -to "${ZITI_ENROLL_TOKEN}"

sudo /opt/openziti/etc/router/bootstrap.bash << ROUTER
ZITI_CTRL_ADVERTISED_ADDRESS="${ZITI_CTRL_ADVERTISED_ADDRESS}"
ZITI_CTRL_ADVERTISED_PORT="${ZITI_CTRL_ADVERTISED_PORT}"
ZITI_ROUTER_ADVERTISED_ADDRESS="${ZITI_ROUTER_ADVERTISED_ADDRESS}"
ZITI_ROUTER_PORT="${ZITI_ROUTER_PORT}"
ZITI_ENROLL_TOKEN="${ZITI_ENROLL_TOKEN}"
ROUTER
sudo systemctl start ziti-router.service
sudo systemd-run \
--wait --quiet \
--service-type=oneshot \
--property=TimeoutStartSec=20s \
systemctl is-active ziti-router.service

ATTEMPTS=10
DELAY=3
until ! ((ATTEMPTS)) || [[ $(ziti edge list edge-routers -j | jq '.data[0].isOnline') == "true" ]]
do
    (( ATTEMPTS-- ))
    echo "INFO: waiting for router to be online"
    sleep ${DELAY}
done
if [[ $(ziti edge list edge-routers -j | jq '.data[0].isOnline') == "true" ]]
then
    echo "INFO: router is online"
else
    echo "INFO: router is offline"
    exit 1
fi

ZITI_CTRL_EDGE_ADVERTISED_ADDRESS=${ZITI_CTRL_ADVERTISED_ADDRESS} \
ZITI_CTRL_EDGE_ADVERTISED_PORT=${ZITI_CTRL_ADVERTISED_PORT} \
go test -v -count=1 -tags="quickstart manual" ./ziti/cmd/edge/...

cleanup
