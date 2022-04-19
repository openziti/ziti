#!/bin/bash

export ZITI_CONTROLLER_RAWNAME=ziti-controller
export ZITI_EDGE_CONTROLLER_RAWNAME=ziti-edge-controller

. "${ZITI_SCRIPTS}/ziti-cli-functions.sh"

generateEnvFile
. ${ZITI_HOME}/ziti.env

# create pki
createPki

# generates the config file for the controller
createControllerConfig

# initialize the database with the admin user:
"${ZITI_BIN_DIR}/ziti-controller" edge init "${ZITI_HOME}/ziti-edge-controller.yaml" -u "${ZITI_USER}" -p "${ZITI_PWD}"

# create a place for the internal db
mkdir -p $ZITI_HOME/db

"${ZITI_BIN_DIR}/ziti-controller" run "${ZITI_HOME}/ziti-edge-controller.yaml"
