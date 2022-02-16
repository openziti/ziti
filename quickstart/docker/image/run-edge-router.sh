#!/bin/bash

. "${ZITI_SCRIPTS}/ziti-cli-functions.sh"

if [[ "${ZITI_EDGE_ROUTER_RAWNAME-}" == "" ]]; then export export ZITI_EDGE_ROUTER_RAWNAME="${ZITI_NETWORK-}-edge-router"; fi
if [[ "${ZITI_EDGE_ROUTER_PORT-}" == "" ]]; then export ZITI_EDGE_ROUTER_PORT="3022"; fi
if [[ "${ZITI_EDGE_ROUTER_HOSTNAME}" == "" ]]; then export ZITI_EDGE_ROUTER_HOSTNAME="${ZITI_EDGE_ROUTER_RAWNAME}${ZITI_DOMAIN_SUFFIX}"; fi

. ${ZITI_HOME}/ziti.env

until $(curl -s -o /dev/null --fail -k "https://${ZITI_EDGE_CTRL_ADVERTISED_HOST_PORT}"); do
    echo "waiting for https://${ZITI_EDGE_CTRL_ADVERTISED_HOST_PORT}"
    sleep 2
done

sleep 2

createRouterPki "${ZITI_EDGE_ROUTER_RAWNAME}"

#echo "logging into ziti controller: ${ZITI_EDGE_CTRL_ADVERTISED_HOST_PORT}"
#"${ZITI_BIN_DIR}/ziti" edge login "${ZITI_EDGE_CTRL_ADVERTISED_HOST_PORT}" -u "${ZITI_USER}" -p "${ZITI_PWD}" -c "${ZITI_PKI}/${ZITI_EDGE_CONTROLLER_ROOTCA_NAME}/certs/${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}.cert"
zitiLogin

if [[ "$1" == "edge" ]]; then
  echo "CREATING EDGE ROUTER CONFIG"
  createEdgeRouterConfig ${ZITI_EDGE_ROUTER_RAWNAME}
fi
if [[ "$1" == "wss" ]]; then
  echo "CREATING EDGE ROUTER WSS CONFIG"
  createEdgeRouterWssConfig ${ZITI_EDGE_ROUTER_RAWNAME}
fi
if [[ "$1" == "fabric" ]]; then
  echo "CREATING FABRIC ROUTER CONFIG"
  createFabricRouterConfig ${ZITI_EDGE_ROUTER_RAWNAME}
fi
if [[ "$1" == "private" ]]; then
  echo "CREATING PRIVATE ROUTER CONFIG"
  createPrivateRouterConfig ${ZITI_EDGE_ROUTER_RAWNAME}
fi

echo "----------  Creating edge-router ${ZITI_EDGE_ROUTER_HOSTNAME}...."
"${ZITI_BIN_DIR}/ziti" edge delete edge-router "${ZITI_EDGE_ROUTER_HOSTNAME}"
"${ZITI_BIN_DIR}/ziti" edge create edge-router "${ZITI_EDGE_ROUTER_HOSTNAME}" -o "${ZITI_HOME}/${ZITI_EDGE_ROUTER_HOSTNAME}.jwt" -t
sleep 1
echo "---------- Enrolling edge-router ${ZITI_EDGE_ROUTER_HOSTNAME}...."
"${ZITI_BIN_DIR}/ziti-router" enroll "${ZITI_HOME}/${ZITI_EDGE_ROUTER_HOSTNAME}.yaml" --jwt "${ZITI_HOME}/${ZITI_EDGE_ROUTER_HOSTNAME}.jwt"
echo ""

"${ZITI_BIN_DIR}/ziti-router" run "${ZITI_HOME}/${ZITI_EDGE_ROUTER_RAWNAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_EDGE_ROUTER_HOSTNAME}.log"

