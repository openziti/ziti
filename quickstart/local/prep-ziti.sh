# if needed, initialize the controller
ziti-controller edge init "${ZITI_HOME}/controller.yaml" -u "${ZITI_USER}" -p "${ZITI_PWD}"

mkdir -p ${ZITI_HOME}

# run the controller
ziti-controller run "${ZITI_HOME}/controller.yaml" > "${ZITI_HOME}/ziti-controller.log" 2>&1 &

# wait for the controller to come up...
sleep 2

ziti edge controller login "${ZITI_EDGE_API_HOSTNAME}" -u "${ZITI_USER}" -p "${ZITI_PWD}" -c "${ZITI_PKI}/${ZITI_EDGE_ROOTCA_NAME}/certs/${ZITI_EDGE_INTERMEDIATE_NAME}.cert"

ziti edge controller create edge-router-policy allEdgeRouters --edge-router-roles '#all' --identity-roles '#all'
ziti edge controller create service-edge-router-policy allSvcRouter --edge-router-roles '#all' --service-roles '#all'

ziti-fabric create router "${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_ROUTER_BR_HOSTNAME}-client.cert"
ziti-fabric create router "${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_ROUTER_BLUE_HOSTNAME}-client.cert"
ziti-fabric create router "${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_ROUTER_RED_HOSTNAME}-client.cert"

echo "---------- Creating  edge-router ${ZITI_EDGE_ROUTER_NAME}...."
ziti edge controller create edge-router "${ZITI_EDGE_ROUTER_NAME}" -o "${ZITI_HOME}/${ZITI_EDGE_ROUTER_NAME}.jwt"
    echo "--- DONE"
    echo ""

echo "---------- Enrolling edge-router ${ZITI_EDGE_ROUTER_NAME}...."
ziti-router enroll "${ZITI_HOME}/${ZITI_EDGE_ROUTER_NAME}.yaml" --jwt "${ZITI_HOME}/${ZITI_EDGE_ROUTER_NAME}.jwt"
    echo "--- DONE"
    echo ""

echo "---------- Creating  edge-router ${ZITI_EDGE_WSS_ROUTER_NAME}...."
ziti edge controller create edge-router "${ZITI_EDGE_WSS_ROUTER_NAME}" -o "${ZITI_HOME}/${ZITI_EDGE_WSS_ROUTER_NAME}.jwt"
    echo "--- DONE"
    echo ""
echo "---------- Enrolling edge-router ${ZITI_EDGE_WSS_ROUTER_NAME}...."
ziti-router enroll "${ZITI_HOME}/${ZITI_EDGE_WSS_ROUTER_NAME}.yaml" --jwt "${ZITI_HOME}/${ZITI_EDGE_WSS_ROUTER_NAME}.jwt"
    echo "--- DONE"
    echo ""
