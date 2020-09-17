
# set a variable to the location of the script running in case it's needed
export curdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
export network_name="${USER}"

. ${curdir}/env.sh

ziti-controller -v run "${ZITI_HOME}/controller.yaml" > "${ZITI_HOME}/ziti-controller.log" 2>&1 &
# sleep 2
# ziti-router run "${ZITI_HOME}/${ZITI_ROUTER_BR_NAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_ROUTER_BR_NAME}.log" 2>&1 &
# sleep 1
# ziti-router run "${ZITI_HOME}/${ZITI_ROUTER_BLUE_NAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_ROUTER_BLUE_NAME}.log" 2>&1 &
sleep 1
ziti-router -v run "${ZITI_HOME}/${ZITI_ROUTER_RED_NAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_ROUTER_RED_NAME}.log" 2>&1 &
# sleep 1
# ziti-router run "${ZITI_HOME}/${ZITI_EDGE_ROUTER_NAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_EDGE_ROUTER_NAME}.log" 2>&1 &
sleep 1
ziti-router -v run "${ZITI_HOME}/${ZITI_EDGE_WSS_ROUTER_NAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_EDGE_WSS_ROUTER_NAME}.log" 2>&1 &
