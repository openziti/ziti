#!/bin/bash

# give the controller time to ramp up before running if running in docker-compose
sleep 5

. "${ZITI_SCRIPTS}/ziti-cli-functions.sh"

if [[ "${ZITI_CONTROLLER_RAWNAME-}" == "" ]]; then export ZITI_CONTROLLER_RAWNAME="ziti-controller"; fi
if [[ "${ZITI_EDGE_CONTROLLER_RAWNAME-}" == "" ]]; then export ZITI_EDGE_CONTROLLER_RAWNAME="ziti-edge-controller"; fi
if [[ "${ZITI_EDGE_ROUTER_RAWNAME-}" == "" ]]; then
  export ZITI_EDGE_ROUTER_DESIRED_RAWNAME="${ZITI_NETWORK-}-edge-router"
else
  ZITI_EDGE_ROUTER_DESIRED_RAWNAME="${ZITI_EDGE_ROUTER_RAWNAME}"
fi
if [[ "${ZITI_EDGE_ROUTER_PORT-}" == "" ]]; then export ZITI_EDGE_ROUTER_PORT="3022"; fi
if [[ "${ZITI_EDGE_ROUTER_HOSTNAME}" == "" ]]; then export ZITI_EDGE_ROUTER_HOSTNAME="${ZITI_EDGE_ROUTER_RAWNAME}${ZITI_DOMAIN_SUFFIX}"; fi
if [[ "${ZITI_EDGE_ROUTER_ROLES}" == "" ]]; then export ZITI_EDGE_ROUTER_ROLES="${ZITI_EDGE_ROUTER_RAWNAME}"; fi

. ${ZITI_HOME}/ziti.env

# This is a unique situation due to the env file being shared among all routers so we need to explicitly set the router
export ZITI_EDGE_ROUTER_RAWNAME="${ZITI_EDGE_ROUTER_DESIRED_RAWNAME}"

# shellcheck disable=SC2091
until $(curl -s -o /dev/null --fail -k "https://${ZITI_EDGE_CTRL_ADVERTISED_HOST_PORT}"); do
    echo "waiting for https://${ZITI_EDGE_CTRL_ADVERTISED_HOST_PORT}"
    sleep 2
done

createRouterPki "${ZITI_EDGE_ROUTER_RAWNAME}"

zitiLogin

if [[ "$1" == "edge" ]]; then
  echo "CREATING EDGE ROUTER CONFIG"
  createEdgeRouterConfig "${ZITI_EDGE_ROUTER_RAWNAME}"
fi
if [[ "$1" == "wss" ]]; then
  echo "CREATING EDGE ROUTER WSS CONFIG"
  createEdgeRouterWssConfig "${ZITI_EDGE_ROUTER_RAWNAME}"
fi
if [[ "$1" == "fabric" ]]; then
  echo "CREATING FABRIC ROUTER CONFIG"
  createFabricRouterConfig "${ZITI_EDGE_ROUTER_RAWNAME}"
fi
if [[ "$1" == "private" ]]; then
  echo "CREATING PRIVATE ROUTER CONFIG"
  createPrivateRouterConfig "${ZITI_EDGE_ROUTER_RAWNAME}"
fi

echo "----------  Creating edge-router ${ZITI_EDGE_ROUTER_HOSTNAME}...."
found=$(ziti edge list edge-routers 'name = "'"${ZITI_EDGE_ROUTER_HOSTNAME}"'"' | grep -c "${ZITI_EDGE_ROUTER_HOSTNAME}")
if [[ found -gt 0 ]]; then
  echo "----------  Found existing edge-router ${ZITI_EDGE_ROUTER_HOSTNAME}...."
else
  "${ZITI_BIN_DIR}/ziti" edge create edge-router "${ZITI_EDGE_ROUTER_HOSTNAME}" -o "${ZITI_HOME}/${ZITI_EDGE_ROUTER_HOSTNAME}.jwt" -t -a "${ZITI_EDGE_ROUTER_ROLES}"
  sleep 1
  echo "---------- Enrolling edge-router ${ZITI_EDGE_ROUTER_HOSTNAME}...."
  "${ZITI_BIN_DIR}/ziti-router" enroll "${ZITI_HOME}/${ZITI_EDGE_ROUTER_HOSTNAME}.yaml" --jwt "${ZITI_HOME}/${ZITI_EDGE_ROUTER_HOSTNAME}.jwt"
  echo ""
fi

"${ZITI_BIN_DIR}/ziti-router" run "${ZITI_HOME}/${ZITI_EDGE_ROUTER_RAWNAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_EDGE_ROUTER_HOSTNAME}.log"

