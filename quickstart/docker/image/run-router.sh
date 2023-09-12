#!/bin/bash

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

  found=$("${ZITI_BIN_DIR-}/ziti" edge list edge-routers 'name = "'"${ZITI_ROUTER_NAME}"'"' | grep -c "${ZITI_ROUTER_NAME}")
  if [[ found -gt 0 ]]; then
    echo "----------  Found existing edge-router ${ZITI_ROUTER_NAME}...."
  else
    "${ZITI_BIN_DIR}/ziti" edge create edge-router "${ZITI_ROUTER_NAME}" -o "${ZITI_HOME}/${ZITI_ROUTER_NAME}.jwt" -t -a "${ZITI_ROUTER_ROLES}"
    sleep 1
    echo "---------- Enrolling edge-router ${ZITI_ROUTER_NAME}...."
    "${ZITI_BIN_DIR}/ziti" router enroll "${ZITI_HOME}/${ZITI_ROUTER_NAME}.yaml" --jwt "${ZITI_HOME}/${ZITI_ROUTER_NAME}.jwt"
    echo ""
  fi
else
    echo " Found existing config file ${_CONFIG_PATH}, not creating a new config."
fi

unset ZITI_USER
unset ZITI_PWD

"${ZITI_BIN_DIR}/ziti" router run "${ZITI_HOME}/${ZITI_ROUTER_NAME}.yaml" > "${ZITI_HOME}/${ZITI_ROUTER_NAME}.log"

