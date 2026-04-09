#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

. "${ZITI_SCRIPTS}/ziti-cli-functions.sh"

if [[ "${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS-}" == "" ]]; then export ZITI_CTRL_EDGE_ADVERTISED_ADDRESS="ziti-edge-controller"; fi
if [[ "${ZITI_CTRL_EDGE_ADVERTISED_PORT-}" == "" ]]; then export ZITI_CTRL_EDGE_ADVERTISED_PORT="1280"; fi
if [[ "${ZITI_ROUTER_PORT-}" == "" ]]; then export ZITI_ROUTER_PORT="3022"; fi
if [[ "${ZITI_ROUTER_ROLES}" == "" ]]; then export ZITI_ROUTER_ROLES="${ZITI_ROUTER_NAME}"; fi

if [[ "${ZITI_ROUTER_NAME-}" != "" ]]; then
  _ZITI_ROUTER_NAME="${ZITI_ROUTER_NAME}"
  echo "_ZITI_ROUTER_NAME set to: ${_ZITI_ROUTER_NAME}"
fi

# Wait until the file exists, then give one more second for the file to be completely written
until [ -f "${ZITI_HOME}/ziti.env" ]
do
  sleep 1
done
sleep 1
. ${ZITI_HOME}/ziti.env

# wait for the controller to come online
_wait_for_controller

# after coming online, give the controller just a second to ramp up in case running via docker compose
sleep 1

if [[ "${_ZITI_ROUTER_NAME}" != "" ]]; then
  export ZITI_ROUTER_NAME="${_ZITI_ROUTER_NAME}"
  echo "ZITI_ROUTER_NAME set to: ${ZITI_ROUTER_NAME}"
fi

_CONFIG_PATH="${ZITI_HOME}/${ZITI_ROUTER_NAME}.yaml"
# Set an error trap to move _CONFIG_PATH when it's unsafe to assume enrollment succeeded
trap '[[ -f "${_CONFIG_PATH}" ]] && mv "${_CONFIG_PATH}" "${_CONFIG_PATH}.err"' ERR

if [ ! -f "${_CONFIG_PATH}" ]; then
  echo "config has not been generated, generating config..."
  "${ZITI_BIN_DIR-}/ziti" edge login ${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS}:${ZITI_CTRL_EDGE_ADVERTISED_PORT} -u $ZITI_USER -p $ZITI_PWD -y

  echo "----------  Creating edge-router ${ZITI_ROUTER_NAME}...."

  if [[ "$1" == "edge" ]]; then
    echo "CREATING EDGE ROUTER CONFIG: ${ZITI_ROUTER_NAME}"
    createEdgeRouterConfig "${ZITI_ROUTER_NAME}"
  fi
  if [[ "$1" == "wss" ]]; then
    echo "CREATING EDGE ROUTER WSS CONFIG: ${ZITI_ROUTER_NAME}"
    createEdgeRouterWssConfig "${ZITI_ROUTER_NAME}"
  fi
  if [[ "$1" == "fabric" ]]; then
    echo "CREATING FABRIC ROUTER CONFIG: ${ZITI_ROUTER_NAME}"
    createFabricRouterConfig "${ZITI_ROUTER_NAME}"
  fi
  if [[ "$1" == "private" ]]; then
    echo "CREATING PRIVATE ROUTER CONFIG: ${ZITI_ROUTER_NAME}"
    createPrivateRouterConfig "${ZITI_ROUTER_NAME}"
  fi

  
  # Retry the edge-router creation in case the controller's Raft cluster hasn't elected a leader yet
  _retries=20
  while ! "${ZITI_BIN_DIR-}/ziti" edge list edge-routers "name = \"${ZITI_ROUTER_NAME}\"" --csv 2>/dev/null | grep -q "${ZITI_ROUTER_NAME}"; do
    if "${ZITI_BIN_DIR}/ziti" edge create edge-router "${ZITI_ROUTER_NAME}" -o "${ZITI_HOME}/${ZITI_ROUTER_NAME}.jwt" -t -a "${ZITI_ROUTER_ROLES}" 2>&1; then
      break
    fi
    if (( --_retries == 0 )); then
      echo "ERROR: failed to create edge-router ${ZITI_ROUTER_NAME} after retries" >&2
      exit 1
    fi
    echo "INFO: waiting for controller to be ready to create edge-router (${_retries} retries left)..."
    sleep 3
  done

  if [ -f "${ZITI_HOME}/${ZITI_ROUTER_NAME}.jwt" ]; then
    echo "---------- Enrolling edge-router ${ZITI_ROUTER_NAME}...."
    "${ZITI_BIN_DIR}/ziti" router enroll "${ZITI_HOME}/${ZITI_ROUTER_NAME}.yaml" --jwt "${ZITI_HOME}/${ZITI_ROUTER_NAME}.jwt"
    echo ""
  else
    echo "----------  Found existing edge-router ${ZITI_ROUTER_NAME}...."
  fi
else
    echo " Found existing config file ${_CONFIG_PATH}, not creating a new config."
fi

unset ZITI_USER
unset ZITI_PWD

"${ZITI_BIN_DIR}/ziti" router run "${ZITI_HOME}/${ZITI_ROUTER_NAME}.yaml"

