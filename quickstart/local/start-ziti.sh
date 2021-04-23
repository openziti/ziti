ziti-router run "${ZITI_HOME}/${ZITI_ROUTER_BR_HOSTNAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_ROUTER_BR_HOSTNAME}.log" 2>&1 &
sleep 1
ziti-router run "${ZITI_HOME}/${ZITI_ROUTER_BLUE_HOSTNAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_ROUTER_BLUE_HOSTNAME}.log" 2>&1 &
sleep 1
ziti-router run "${ZITI_HOME}/${ZITI_ROUTER_RED_HOSTNAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_ROUTER_RED_HOSTNAME}.log" 2>&1 &
sleep 1
ziti-router run "${ZITI_HOME}/${ZITI_EDGE_ROUTER_HOSTNAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_EDGE_ROUTER_HOSTNAME}.log" 2>&1 &
sleep 1
ziti-router run "${ZITI_HOME}/${ZITI_EDGE_WSS_ROUTER_HOSTNAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_EDGE_WSS_ROUTER_HOSTNAME}.log" 2>&1 &
