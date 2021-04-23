#!/bin/bash
. ${HOME}/ziti.env
echo " "
echo " "
cat ${HOME}/ziti.env
echo " "
echo " "

echo "logging into ziti controller: ${ZITI_EDGE_API_HOSTNAME}"
echo ziti edge controller login "${ZITI_EDGE_API_HOSTNAME}" -u "${ZITI_USER}" -p "${ZITI_PWD}" -c "${ZITI_PKI}/${ZITI_EDGE_ROOTCA_NAME}/certs/${ZITI_EDGE_INTERMEDIATE_NAME}.cert"
ziti edge controller login "${ZITI_EDGE_API_HOSTNAME}" -u "${ZITI_USER}" -p "${ZITI_PWD}" -c "${ZITI_PKI}/${ZITI_CONTROLLER_ROOTCA_NAME}/certs/${ZITI_CONTROLLER_INTERMEDIATE_NAME}.cert"

<<<<<<< Updated upstream
=======
# create a new gateway
ziti edge controller create edge-router "${ZITI_EDGE_CONTROLLER_HOSTNAME}" "${ZITI_CLUSTER_NAME}" -o "${ZITI_HOME}/${ZITI_EDGE_CONTROLLER_HOSTNAME}.jwt"
>>>>>>> Stashed changes

echo "HOSTNAME: $(hostname)"
echo "CREATING CONFIG"
"${HOME}/create-edge-router-config.sh"

<<<<<<< Updated upstream
echo "----------  Creating edge-router ${ZITI_EDGE_ROUTER_NAME}...."
ziti edge controller create edge-router "${ZITI_EDGE_ROUTER_NAME}" -o "${ZITI_HOME}/${ZITI_EDGE_ROUTER_NAME}.jwt" -t
sleep 1
echo "---------- Enrolling edge-router ${ZITI_EDGE_ROUTER_NAME}...."
ziti-router enroll "${ZITI_HOME}/${ZITI_EDGE_ROUTER_NAME}.yaml" --jwt "${ZITI_HOME}/${ZITI_EDGE_ROUTER_NAME}.jwt" &> "${ZITI_EDGE_ROUTER_NAME}.enrollment.log"
echo ""
sleep 1
ziti-router run "${ZITI_HOME}/${ZITI_EDGE_ROUTER_NAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_EDGE_ROUTER_NAME}.log"
=======
# race condition?
sleep 2

# enroll the gateway
ziti-router enroll "${ZITI_HOME}/${ZITI_EDGE_ROUTER_NAME}.yaml" --jwt "${ZITI_HOME}/${ZITI_EDGE_CONTROLLER_HOSTNAME}.jwt"

# give the register process a moment to breath. it's hypthesized that there's a race condition
# between register and running so let it sit for a moment...
sleep 2

# start the gateway
ziti-router run "${ZITI_HOME}/${ZITI_EDGE_ROUTER_NAME}.yaml"
