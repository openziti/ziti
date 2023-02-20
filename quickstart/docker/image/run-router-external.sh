#!/bin/bash

# give the controller time to ramp up before running if running in docker-compose
echo "Waiting for 5 seconds...."
sleep 5

. "${ZITI_SCRIPTS}/ziti-cli-functions.sh"

# If we have not defined these, then no point going further - so dont
if [[ "${ZITI_EDGE_ROUTER_HOSTNAME}" == "" ]]; then echo "ERROR: Missing ZITI_EDGE_ROUTER_HOSTNAME definition" >&2; exit; fi
export ZITI_EDGE_ROUTER_RAWNAME=${ZITI_EDGE_ROUTER_RAWNAME_OVERRIDE}

echo "ZITI_EDGE_ROUTER_HOSTNAME = ${ZITI_EDGE_ROUTER_HOSTNAME}"


# If we dont have a key file, then we assume we haven't enrolled, so lets do that
if [ ! -f ${ZITI_EDGE_ROUTER_HOSTNAME}.jwt ]; then

  # May have been a bad attempt before, so lets clean up
  rm -f ${ZITI_HOME}/${ZITI_EDGE_ROUTER_HOSTNAME}*


  # If we have override variables then override the base variable
  # This is required as the .bashrc loads the env file which clobbers passed in varibles
  if [[ "${ZITI_EDGE_ROUTER_RAWNAME_OVERRIDE}" == "" ]]; then echo "ERROR: Missing ZITI_EDGE_ROUTER_RAWNAME_OVERRIDE definition" >&2; exit; fi
    # May have been a bad attempt before, so lets clean up
  rm -f ${ZITI_HOME}/${ZITI_EDGE_ROUTER_RAWNAME}*

  if [[ ${ZITI_CTRL_ADVERTISED_ADDRESS_OVERRIDE} != "" ]]; then export ZITI_CTRL_ADVERTISED_ADDRESS=${ZITI_CTRL_ADVERTISED_ADDRESS_OVERRIDE}; fi
  if [[ ${ZITI_USER_OVERRIDE} != "" ]]; then export ZITI_USER=${ZITI_USER_OVERRIDE}; fi
  if [[ ${ZITI_PWD_OVERRIDE} != "" ]]; then export ZITI_PWD=${ZITI_PWD_OVERRIDE}; fi
  if [[ ${ZITI_EDGE_CONTROLLER_PORT_OVERRIDE} != "" ]]; then export ZITI_EDGE_CONTROLLER_PORT=${ZITI_EDGE_CONTROLLER_PORT_OVERRIDE}; fi
  if [[ ${ZITI_EDGE_ROUTER_HOSTNAME} == "" ]]; then export ZITI_EDGE_ROUTER_HOSTNAME=${ZITI_EDGE_ROUTER_RAWNAME}; fi

  # First off - lets make sure we have what we need
  if [[ ${ZITI_USER} == "" ]] || [[ ${ZITI_PWD} == "" ]]; then echo "ERROR:  Need ZITI_USER and ZITI_PWD defined" >&2; exit; fi
  if [[ "$1" == "" ]]; then echo "ERROR:  Have not defined router type.  It should be one of edge,wss,fabric or private." >&2; exit; fi
  if [[ "${ZITI_CTRL_ADVERTISED_ADDRESS}" == "" ]]; then echo "ERROR: Missing ZITI_CTRL_ADVERTISED_ADDRESS_OVERRIDE definition" >&2; exit; fi
  if [[ "${ZITI_EDGE_ROUTER_PORT-}" == "" ]]; then export ZITI_EDGE_ROUTER_PORT="3022"; fi
  if [[ "${ZITI_EDGE_ROUTER_ROLES}" == "" ]]; then export ZITI_EDGE_ROUTER_ROLES="${ZITI_EDGE_ROUTER_HOSTNAME}"; fi

  echo "ZITI_EDGE_ROUTER_RAWNAME = ${ZITI_EDGE_ROUTER_RAWNAME}"
  echo "ZITI_EDGE_ROUTER_ROLES = ${ZITI_EDGE_ROUTER_ROLES}"
  echo "ZITI_EDGE_ROUTER_PORT = ${ZITI_EDGE_ROUTER_PORT}"
  echo "ZITI_CTRL_ADVERTISED_ADDRESS = ${ZITI_CTRL_ADVERTISED_ADDRESS}"
  echo "ZITI_CTRL_PORT = ${ZITI_CTRL_PORT}"
  echo "ZITI_USER = ${ZITI_USER}"
  if [[ ! "${ZITI_PWD}" == "" ]]; then echo "ZITI_PWD = (obscured)"; fi

  # Login to the cloud controller
  "${ZITI_BIN_DIR}/ziti" edge login -y ${ZITI_CTRL_ADVERTISED_ADDRESS}:${ZITI_EDGE_CONTROLLER_PORT} -u ${ZITI_USER} -p ${ZITI_PWD}

  if [[ "$1" == "edge" ]]; then
    echo "CREATING EDGE ROUTER CONFIG"
    createEdgeRouterConfig "${ZITI_EDGE_ROUTER_RAWNAME}"
  fi
  if [[ "$1" == "wss" ]]; then
    echo "CREATING EDGE ROUTER WSS CONFIG"
    createEdgeRouterWssConfig "${ZITI_EDGE_ROUTER_RAWNAME}"
  fi
  if [[ "$1" == "fabric" ]]; then
    echo "CREATING FABRIC ROUTER CONFIG"
    createFabricRouterConfig "${ZITI_EDGE_ROUTER_RAWNAME}"
  fi
  if [[ "$1" == "private" ]]; then
    echo "CREATING PRIVATE ROUTER CONFIG"
    createPrivateRouterConfig "${ZITI_EDGE_ROUTER_RAWNAME}"
  fi

  mv ${ZITI_HOME}/${ZITI_EDGE_ROUTER_RAWNAME}.yaml ${ZITI_HOME}/${ZITI_EDGE_ROUTER_HOSTNAME}.yaml

  echo "----------  Creating edge-router ${ZITI_EDGE_ROUTER_RAWNAME}...."
  found=$(${ZITI_BIN_DIR}/ziti edge list edge-routers 'name = "'"${ZITI_EDGE_ROUTER_HOSTNAME}"'"' | grep -c "${ZITI_EDGE_ROUTER_HOSTNAME}")
  if [[ found -gt 0 ]]; then
    echo "----------  Found existing edge-router ${ZITI_EDGE_ROUTER_HOSTNAME}...."
  else
    "${ZITI_BIN_DIR}/ziti" edge create edge-router "${ZITI_EDGE_ROUTER_HOSTNAME}" -o "${ZITI_HOME}/${ZITI_EDGE_ROUTER_HOSTNAME}.jwt" -t -a "${ZITI_EDGE_ROUTER_ROLES}"
    sleep 1
    echo "---------- Enrolling edge-router ${ZITI_EDGE_ROUTER_HOSTNAME}...."
  "${ZITI_BIN_DIR}/ziti-router" enroll "${ZITI_HOME}/${ZITI_EDGE_ROUTER_HOSTNAME}.yaml" --jwt "${ZITI_HOME}/${ZITI_EDGE_ROUTER_HOSTNAME}.jwt"
    echo ""
  fi
fi

# Run the router
"${ZITI_BIN_DIR}/ziti-router" run "${ZITI_HOME}/${ZITI_EDGE_ROUTER_HOSTNAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_EDGE_ROUTER_HOSTNAME}.log"

