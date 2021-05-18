#!/bin/bash

. "${ZITI_SCRIPTS}/ziti-cli-functions.sh"

ziti_createEnvFile
. ${ZITI_HOME}/ziti.env

# create pki
createPki

# generates the config file for the controller
createControllerConfig

# initialize the database with the admin user:
"${ZITI_BIN_DIR}/ziti-controller" edge init "${ZITI_HOME}/controller.yaml" -u "${ZITI_USER}" -p "${ZITI_PWD}"

# create a place for the internal db
mkdir -p $ZITI_HOME/db

#"${ZITI_SCRIPTS}/create-fabric-identity.sh"
createFabricIdentity

"${ZITI_BIN_DIR}/ziti-controller" run "${ZITI_HOME}/controller.yaml"

${ZITI_HOME}/pki/ziti-controller-intermediate/keys/ziti-dotzeet.key
