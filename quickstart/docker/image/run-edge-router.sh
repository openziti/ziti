#!/bin/bash
. /openziti/ziti.env

#until $(curl --output /dev/null --silent --head --fail "${ZITI_EDGE_CONTROLLER_API}"); do
#until $(curl -s -o /dev/null -w "%{http_code}" -k --fail "https://${ZITI_EDGE_CONTROLLER_API}"); do
until $(curl -s -o /dev/null --fail -k "https://${ZITI_EDGE_CONTROLLER_API}"); do
    echo "waiting for https://${ZITI_EDGE_CONTROLLER_API}"
    sleep 2
done

ziti edge controller login "${ZITI_EDGE_CONTROLLER_API}" -u "${ZITI_USER}" -p "${ZITI_PWD}" -c "${ZITI_PKI}/${ZITI_EDGE_CONTROLLER_ROOTCA_NAME}/certs/${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}.cert"

# create a new gateway
ziti edge controller create edge-router "${ZITI_EDGE_CONTROLLER_NAME}" "${ZITI_CLUSTER_NAME}" -o "${ZITI_HOME}/${ZITI_EDGE_CONTROLLER_NAME}.jwt"

"${HOME}/create-edge-router-config.sh"

# race condition?
sleep 2

# enroll the gateway
ziti-router enroll "${ZITI_HOME}/${ZITI_EDGE_ROUTER_NAME}.yaml" --jwt "${ZITI_HOME}/${ZITI_EDGE_CONTROLLER_NAME}.jwt"

# give the register process a moment to breath. it's hypthesized that there's a race condition
# between register and running so let it sit for a moment...
sleep 2

# start the gateway
ziti-router run "${ZITI_HOME}/${ZITI_EDGE_ROUTER_NAME}.yaml"
