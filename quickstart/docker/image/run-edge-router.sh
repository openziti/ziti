#!/bin/bash
. /openziti/ziti.env

until $(curl -s -o /dev/null --fail -k "https://${ZITI_EDGE_CONTROLLER_API}"); do
    echo "waiting for https://${ZITI_EDGE_CONTROLLER_API}"
    sleep 2
done

echo "logging into ziti controller: ${ZITI_EDGE_API_HOSTNAME}"
ziti edge login "${ZITI_EDGE_CONTROLLER_API}" -u "${ZITI_USER}" -p "${ZITI_PWD}" -c "${ZITI_PKI}/${ZITI_EDGE_CONTROLLER_ROOTCA_NAME}/certs/${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}.cert"

echo "CREATING CONFIG"
"${HOME}/create-edge-router-config.sh"

echo "----------  Creating edge-router ${ZITI_EDGE_ROUTER_NAME}...."
ziti edge controller create edge-router "${ZITI_EDGE_ROUTER_NAME}" -o "${ZITI_HOME}/${ZITI_EDGE_ROUTER_NAME}.jwt" -t
sleep 1
echo "---------- Enrolling edge-router ${ZITI_EDGE_ROUTER_NAME}...."
ziti-router enroll "${ZITI_HOME}/${ZITI_EDGE_ROUTER_NAME}.yaml" --jwt "${ZITI_HOME}/${ZITI_EDGE_ROUTER_NAME}.jwt" &> "${ZITI_EDGE_ROUTER_NAME}.enrollment.log"
echo ""
sleep 1
ziti-router run "${ZITI_HOME}/${ZITI_EDGE_ROUTER_NAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_EDGE_ROUTER_NAME}.log"
