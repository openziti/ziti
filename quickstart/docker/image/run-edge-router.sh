#!/bin/bash

echo 'export ZITI_EDGE_ROUTER_RAWNAME="'"${ZITI_EDGE_ROUTER_RAWNAME}"'"' >> ziti.env
echo 'export ZITI_EDGE_ROUTER_HOSTNAME="'"${ZITI_EDGE_ROUTER_RAWNAME}${ZITI_DOMAIN_SUFFIX}"'"' >> ziti.env
. /openziti/ziti.env

until $(curl -s -o /dev/null --fail -k "https://${ZITI_EDGE_CONTROLLER_API}"); do
    echo "waiting for https://${ZITI_EDGE_CONTROLLER_API}"
    sleep 2
done

if [[ "$1" == "wss" ]]; then
  echo "CREATING WSS EDGE ROUTER"
  "/openziti/create-wss-router-pki.sh"
else
  echo "CREATING EDGE ROUTER"
  "/openziti/create-router-pki.sh"
fi

echo "logging into ziti controller: ${ZITI_EDGE_API_HOSTNAME}"
ziti edge login "${ZITI_EDGE_CONTROLLER_API}" -u "${ZITI_USER}" -p "${ZITI_PWD}" -c "${ZITI_PKI}/${ZITI_EDGE_CONTROLLER_ROOTCA_NAME}/certs/${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}.cert"

echo "CREATING CONFIG"
#sleep 1000
"/openziti/create-edge-router-config.sh"

echo "----------  Creating edge-router ${ZITI_EDGE_ROUTER_HOSTNAME}...."
ziti edge delete edge-router "${ZITI_EDGE_ROUTER_HOSTNAME}"
ziti edge create edge-router "${ZITI_EDGE_ROUTER_HOSTNAME}" -o "${ZITI_HOME}/${ZITI_EDGE_ROUTER_HOSTNAME}.jwt" -t
sleep 1
echo "---------- Enrolling edge-router ${ZITI_EDGE_ROUTER_HOSTNAME}...."
ziti-router enroll "${ZITI_HOME}/${ZITI_EDGE_ROUTER_HOSTNAME}.yaml" --jwt "${ZITI_HOME}/${ZITI_EDGE_ROUTER_HOSTNAME}.jwt"
echo ""
sleep 1
ziti-router run "${ZITI_HOME}/${ZITI_EDGE_ROUTER_HOSTNAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_EDGE_ROUTER_HOSTNAME}.log"

