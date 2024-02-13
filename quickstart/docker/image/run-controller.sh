#!/bin/bash
ziti_controller_cfg="${ZITI_HOME}/ziti-edge-controller.yaml"

# Global Variables
ASCI_RESTORE='\033[0m'
ASCI_RED='\033[00;31m'

function RED {  # Generally used for ERROR
  echo "${ASCI_RED}${1-}${ASCI_RESTORE}"
}

if [[ "${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS-}" == "" ]]; then export ZITI_CTRL_EDGE_ADVERTISED_ADDRESS="ziti-edge-controller"; fi
if [[ "${ZITI_CTRL_NAME-}" == "" ]]; then export ZITI_CTRL_NAME="${ZITI_NETWORK}-controller"; fi

. "${ZITI_SCRIPTS}/ziti-cli-functions.sh"

if [ ! -f "${ZITI_HOME}/access-control.init" ]; then
  echo "system has not been initialized. initializing..."
  setupEnvironment
  persistEnvironmentValues
  # don't move the sourcing of the file. yes it's duplicated but it needs to be here
  . ${ZITI_HOME}/ziti.env

  if [ ! -f "${ZITI_HOME}/access-control.init" ]; then
    setupEnvironment
    persistEnvironmentValues
  else
    echo "system has been initialized already. just starting the process"
  fi
  # create pki
  createPki
  if [ ! -f "${ziti_controller_cfg}" ]; then
    echo " "
    echo "${ziti_controller_cfg} doesn't exist. Generating config file"
    echo " "
    # generates the config file for the controller
    createControllerConfig
  else
    echo " "
    echo "${ziti_controller_cfg} exists. Not overwriting"
    echo " "
  fi

  # initialize the database with the admin user:
  "${ZITI_BIN_DIR}/ziti" controller edge init "${ZITI_HOME}/${ZITI_CTRL_NAME}.yaml" -u "${ZITI_USER}" -p "${ZITI_PWD}"
  if [[ "$?" != 0 ]]; then
    echo -e "$(RED "  --- There was an error while initializing the controller ---")"
    exit 1
  fi
else
  echo "system has been initialized. starting the process."
  # don't move the sourcing of the file. yes it's duplicated but it needs to be here
  . ${ZITI_HOME}/ziti.env
fi

echo "controller initialized. unsetting ZITI_USER/ZITI_PWD from env"
unset ZITI_USER
unset ZITI_PWD

# create a place for the internal db
mkdir -p $ZITI_HOME/db

"${ZITI_BIN_DIR}/ziti" controller run "${ZITI_HOME}/${ZITI_CTRL_NAME}.yaml"
