# if needed, initialize the controller
ziti-controller edge init "${ZITI_HOME}/controller.yaml" -u "${ZITI_USER}" -p "${ZITI_PWD}"

# tail all the log files that will be created to see any issues
mkdir -p ${ZITI_HOME}
tail -F ${ZITI_HOME}/ziti-*log &

# run the controller
ziti-controller run "${ZITI_HOME}/controller.yaml" > "${ZITI_HOME}/ziti-controller.log" 2>&1 &

# wait for the controller to come up...
sleep 2

ziti edge controller login "${ZITI_EDGE_API_HOSTNAME}" -u "${ZITI_USER}" -p "${ZITI_PWD}" -c "${ZITI_PKI}/${ZITI_EDGE_ROOTCA_NAME}/certs/${ZITI_EDGE_INTERMEDIATE_NAME}.cert"

ziti edge controller create cluster "${ZITI_CLUSTER_NAME}"

ziti-fabric create router "${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_ROUTER_BR_HOSTNAME}-client.cert"
ziti-fabric create router "${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_ROUTER_BLUE_HOSTNAME}-client.cert"
ziti-fabric create router "${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_ROUTER_RED_HOSTNAME}-client.cert"

ziti edge controller create edge-router "${ZITI_EDGE_ROUTER_NAME}" "${ZITI_CLUSTER_NAME}" -o "${ZITI_HOME}/${ZITI_EDGE_ROUTER_NAME}.jwt"
ziti-router enroll "${ZITI_HOME}/${ZITI_EDGE_ROUTER_NAME}.yaml" --jwt "${ZITI_HOME}/${ZITI_EDGE_ROUTER_NAME}.jwt"
