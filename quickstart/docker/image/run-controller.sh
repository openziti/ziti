#!/bin/bash

. ${ZITI_SCRIPTS}/env.sh
. ${ZITI_HOME}/ziti.env

"${ZITI_SCRIPTS}/create-pki.sh"

# create pki

# generates the config file for the controller
"${ZITI_SCRIPTS}/create-controller-config.sh"

# initialize the database with the admin user:
ziti-controller edge init "${ZITI_HOME}/controller.yaml" -u "${ZITI_USER}" -p "${ZITI_PWD}"

# create a place for the internal db
mkdir -p $ZITI_HOME/db

cat > ${ZITI_HOME}/identities.yml <<IdentitiesJsonHereDoc
---
default:
  caCert:   "${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_CONTROLLER_HOSTNAME}-server.chain.pem"
  cert:     "${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_NETWORK}-dotzeet.cert"
  key:      "${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/keys/${ZITI_NETWORK}-dotzeet.key"
  endpoint: tls:${ZITI_CONTROLLER_HOSTNAME}:${ZITI_FAB_MGMT_PORT}
IdentitiesJsonHereDoc


ziti-controller run "${ZITI_HOME}/controller.yaml"

${ZITI_HOME}/pki/ziti-controller-intermediate/keys/ziti-dotzeet.key