ziti-router run "${ZITI_HOME}/${ZITI_ROUTER_BR_NAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_ROUTER_BR_NAME}.log" 2>&1 &
sleep 1
ziti-router run "${ZITI_HOME}/${ZITI_ROUTER_BLUE_NAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_ROUTER_BLUE_NAME}.log" 2>&1 &
sleep 1
ziti-router run "${ZITI_HOME}/${ZITI_ROUTER_RED_NAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_ROUTER_RED_NAME}.log" 2>&1 &
sleep 1
ziti-router run "${ZITI_HOME}/${ZITI_EDGE_ROUTER_NAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_EDGE_ROUTER_NAME}.log" 2>&1 &
sleep 1
ziti-router run "${ZITI_HOME}/${ZITI_EDGE_WSS_ROUTER_NAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_EDGE_WSS_ROUTER_NAME}.log" 2>&1 &
