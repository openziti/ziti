#!/bin/bash

. /openziti/ziti.env

until $(curl -s -o /dev/null --fail -k "https://${ZITI_EDGE_CONTROLLER_API}"); do
    echo "waiting for https://${ZITI_EDGE_CONTROLLER_API}"
    sleep 2
done

export ZITI_EDGE_ROUTER_HOSTNAME="${ZITI_EDGE_ROUTER_RAWNAME}${ZITI_DOMAIN_SUFFIX}"

#echo 'export ZITI_EDGE_ROUTER_RAWNAME="'"${ZITI_EDGE_ROUTER_RAWNAME}"'"' > "${ZITI_EDGE_ROUTER_HOSTNAME}.env"
#echo 'export ZITI_EDGE_ROUTER_HOSTNAME="'"${ZITI_EDGE_ROUTER_RAWNAME}${ZITI_DOMAIN_SUFFIX}"'"' >> "${ZITI_EDGE_ROUTER_HOSTNAME}.env"

"/openziti/create-router-pki.sh"

echo "logging into ziti controller: ${ZITI_EDGE_API_HOSTNAME}"
ziti edge login "${ZITI_EDGE_CONTROLLER_API}" -u "${ZITI_USER}" -p "${ZITI_PWD}" -c "${ZITI_PKI}/${ZITI_EDGE_CONTROLLER_ROOTCA_NAME}/certs/${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}.cert"

if [[ "$1" == "edge" ]]; then
  echo "CREATING EDGE ROUTER CONFIG"
  "/openziti/create-edge-router-config.sh"
fi
if [[ "$1" == "wss" ]]; then
  echo "CREATING EDGE ROUTER WSS CONFIG"
  "/openziti/create-edge-router-wss-config.sh"
fi
if [[ "$1" == "fabric" ]]; then
  echo "CREATING FABRIC ROUTER CONFIG"
  "/openziti/create-fabric-router-config.sh"
fi
if [[ "$1" == "private" ]]; then
  echo "CREATING PRIVATE ROUTER CONFIG"
  "/openziti/create-fabric-router-config.sh"
fi

echo "----------  Creating edge-router ${ZITI_EDGE_ROUTER_HOSTNAME}...."
ziti edge delete edge-router "${ZITI_EDGE_ROUTER_HOSTNAME}"
ziti edge create edge-router "${ZITI_EDGE_ROUTER_HOSTNAME}" -o "${ZITI_HOME}/${ZITI_EDGE_ROUTER_HOSTNAME}.jwt" -t
sleep 1
echo "---------- Enrolling edge-router ${ZITI_EDGE_ROUTER_HOSTNAME}...."
ziti-router enroll "${ZITI_HOME}/${ZITI_EDGE_ROUTER_HOSTNAME}.yaml" --jwt "${ZITI_HOME}/${ZITI_EDGE_ROUTER_HOSTNAME}.jwt"
echo ""
#sleep 100000
ziti-router run "${ZITI_HOME}/${ZITI_EDGE_ROUTER_HOSTNAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_EDGE_ROUTER_HOSTNAME}.log"

