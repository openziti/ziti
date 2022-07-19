#!/bin/bash

ziti_controller_cfg='/openziti/ziti-edge-controller.yaml'

if [[ "${ZITI_CONTROLLER_HOSTNAME-}" == "" ]]; then export export ZITI_CONTROLLER_HOSTNAME="ziti-controller"; fi
if [[ "${ZITI_EDGE_CONTROLLER_HOSTNAME-}" == "" ]]; then export export ZITI_EDGE_CONTROLLER_HOSTNAME="ziti-edge-controller"; fi

. "${ZITI_SCRIPTS}/ziti-cli-functions.sh"

generateEnvFile
. "${ZITI_HOME}/ziti.env"

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
"${ZITI_BIN_DIR}/ziti-controller" edge init "${ZITI_HOME}/${ZITI_EDGE_CONTROLLER_HOSTNAME}.yaml" -u "${ZITI_USER}" -p "${ZITI_PWD}"

# create a place for the internal db
mkdir -p "$ZITI_HOME/db"

"${ZITI_BIN_DIR}/ziti-controller" run "${ZITI_HOME}/${ZITI_EDGE_CONTROLLER_HOSTNAME}.yaml"
