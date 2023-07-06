#!/bin/bash

. "${ZITI_SCRIPTS}/ziti-cli-functions.sh"

# wait for the controller to come online
_wait_for_controller

# after coming online, give the controller just a second to ramp up in case running via docker compose
sleep 1

if [[ "${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS-}" == "" ]]; then export ZITI_CTRL_EDGE_ADVERTISED_ADDRESS="ziti-edge-controller"; fi
if [[ "${ZITI_CTRL_EDGE_ADVERTISED_PORT-}" == "" ]]; then export ZITI_CTRL_EDGE_ADVERTISED_PORT="3022"; fi
if [[ "${ZITI_EDGE_ROUTER_PORT-}" == "" ]]; then export ZITI_EDGE_ROUTER_PORT="3022"; fi
if [[ "${ZITI_EDGE_ROUTER_ROLES}" == "" ]]; then export ZITI_EDGE_ROUTER_ROLES="${ZITI_EDGE_ROUTER_NAME}"; fi

if [[ "${ZITI_EDGE_ROUTER_NAME-}" != "" ]]; then
  _ZITI_EDGE_ROUTER_NAME="${ZITI_EDGE_ROUTER_NAME}"
  echo "_ZITI_EDGE_ROUTER_NAME set to: ${_ZITI_EDGE_ROUTER_NAME}"
fi

. ${ZITI_HOME}/ziti.env

if [[ "${_ZITI_EDGE_ROUTER_NAME}" != "" ]]; then
  export ZITI_EDGE_ROUTER_NAME="${_ZITI_EDGE_ROUTER_NAME}"
  echo "ZITI_EDGE_ROUTER_NAME set to: ${ZITI_EDGE_ROUTER_NAME}"
fi
_UNIQUE_NAME="${ZITI_HOME}/${ZITI_EDGE_ROUTER_NAME}-${HOSTNAME}.init"
if [ ! -f "${_UNIQUE_NAME}" ]; then
  echo "system has not been initialized. initializing..."
  ziti edge login ${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS}:${ZITI_CTRL_EDGE_ADVERTISED_PORT} -u $ZITI_USER -p $ZITI_PWD -y

  echo "----------  Creating edge-router ${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS}...."

  if [[ "$1" == "edge" ]]; then
    echo "CREATING EDGE ROUTER CONFIG: ${ZITI_EDGE_ROUTER_NAME}"
    createEdgeRouterConfig "${ZITI_EDGE_ROUTER_NAME}"
  fi
  if [[ "$1" == "wss" ]]; then
    echo "CREATING EDGE ROUTER WSS CONFIG: ${ZITI_EDGE_ROUTER_NAME}"
    createEdgeRouterWssConfig "${ZITI_EDGE_ROUTER_NAME}"
  fi
  if [[ "$1" == "fabric" ]]; then
    echo "CREATING FABRIC ROUTER CONFIG: ${ZITI_EDGE_ROUTER_NAME}"
    createFabricRouterConfig "${ZITI_EDGE_ROUTER_NAME}"
  fi
  if [[ "$1" == "private" ]]; then
    echo "CREATING PRIVATE ROUTER CONFIG: ${ZITI_EDGE_ROUTER_NAME}"
    createPrivateRouterConfig "${ZITI_EDGE_ROUTER_NAME}"
  fi

  found=$(ziti edge list edge-routers 'name = "'"${ZITI_EDGE_ROUTER_NAME}"'"' | grep -c "${ZITI_EDGE_ROUTER_NAME}")
  if [[ found -gt 0 ]]; then
    echo "----------  Found existing edge-router ${ZITI_EDGE_ROUTER_NAME}...."
  else
    "${ZITI_BIN_DIR}/ziti" edge create edge-router "${ZITI_EDGE_ROUTER_NAME}" -o "${ZITI_HOME}/${ZITI_EDGE_ROUTER_NAME}.jwt" -t -a "${ZITI_EDGE_ROUTER_ROLES}"
    sleep 1
    echo "---------- Enrolling edge-router ${ZITI_EDGE_ROUTER_NAME}...."
    "${ZITI_BIN_DIR}/ziti" router enroll "${ZITI_HOME}/${ZITI_EDGE_ROUTER_NAME}.yaml" --jwt "${ZITI_HOME}/${ZITI_EDGE_ROUTER_NAME}.jwt"
    echo ""
  fi
  echo "system initialized. writing marker file"
  echo "system initialized" > "${_UNIQUE_NAME}"
else
  echo "system has been initialized. starting the process."
fi

unset ZITI_USER
unset ZITI_PWD

"${ZITI_BIN_DIR}/ziti" router run "${ZITI_HOME}/${ZITI_EDGE_ROUTER_NAME}.yaml" > "${ZITI_HOME}/${ZITI_EDGE_ROUTER_NAME}.log"

