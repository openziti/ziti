#!/usr/bin/env bash
set -euo pipefail

# set a variable to the location of the script running in case it's needed
export curdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
if [[ $# -ne 0 ]]; then
    export network_name=$1
else
    export network_name="${USER}"
fi

ziti_createEnvFile

DIRNAME=$(dirname $0)
[[ ${USE_DNSMASQ:-} -eq 1 ]] && source ${DIRNAME}/run-dns.sh

"${ZITI_BIN_DIR}/ziti-controller" -v run "${ZITI_HOME}/controller.yaml" > "${ZITI_HOME}/ziti_edge-controller.log" 2>&1 &
# sleep 2
# ziti-router run "${ZITI_HOME}/${ZITI_ROUTER_BR_NAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_ROUTER_BR_NAME}.log" 2>&1 &
# sleep 1
# ziti-router run "${ZITI_HOME}/${ZITI_ROUTER_BLUE_NAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_ROUTER_BLUE_NAME}.log" 2>&1 &
sleep 1
ziti-router -v run "${ZITI_HOME}/${ZITI_ROUTER_RED_NAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_ROUTER_RED_NAME}.log" 2>&1 &
# sleep 1
# ziti-router run "${ZITI_HOME}/${ZITI_ROUTER_ADVERTISED_ADDRESS}.yaml" > "${ZITI_HOME}/ziti-${ZITI_ROUTER_ADVERTISED_ADDRESS}.log" 2>&1 &
sleep 1
ziti-router -v run "${ZITI_HOME}/${ZITI_EDGE_WSS_ROUTER_HOSTNAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_EDGE_WSS_ROUTER_HOSTNAME}.log" 2>&1 &
