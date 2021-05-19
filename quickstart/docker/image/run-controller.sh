#!/bin/bash

. ${ZITI_SCRIPTS}/env.sh
. ${ZITI_HOME}/ziti.env

"${ZITI_SCRIPTS}/create-pki.sh"

# create pki

# generates the config file for the controller
"${ZITI_SCRIPTS}/create-controller-config.sh"

# initialize the database with the admin user:
"${ZITI_BIN_DIR}/ziti-controller" edge init "${ZITI_HOME}/controller.yaml" -u "${ZITI_USER}" -p "${ZITI_PWD}"

# create a place for the internal db
mkdir -p $ZITI_HOME/db

"${ZITI_SCRIPTS}/create-fabric-identity.sh"

"${ZITI_BIN_DIR}/ziti-controller" run "${ZITI_HOME}/controller.yaml"

${ZITI_HOME}/pki/ziti-controller-intermediate/keys/ziti-dotzeet.key