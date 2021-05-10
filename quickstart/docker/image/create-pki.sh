#!/bin/bash

source ${ZITI_SCRIPTS}/pki-functions.sh

pki_create_ca "${ZITI_CONTROLLER_ROOTCA_NAME}"
pki_create_ca "${ZITI_EDGE_CONTROLLER_ROOTCA_NAME}"
pki_create_ca "${ZITI_SIGNING_ROOTCA_NAME}"

ZITI_SPURIOUS_INTERMEDIATE="${ZITI_SIGNING_INTERMEDIATE_NAME}_spurious_intermediate"
pki_create_intermediate "${ZITI_CONTROLLER_ROOTCA_NAME}" "${ZITI_CONTROLLER_INTERMEDIATE_NAME}" 1
pki_create_intermediate "${ZITI_EDGE_CONTROLLER_ROOTCA_NAME}" "${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}" 1
pki_create_intermediate "${ZITI_SIGNING_ROOTCA_NAME}" "${ZITI_SPURIOUS_INTERMEDIATE}" 2
pki_create_intermediate "${ZITI_SPURIOUS_INTERMEDIATE}" "${ZITI_SIGNING_INTERMEDIATE_NAME}" 1

if ! test -f "${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/keys/${ZITI_NETWORK}-dotzeet.key"; then
  echo "Creating ziti-fabric client certificate for network: ${ZITI_NETWORK}"
  ziti pki create client --pki-root="${ZITI_PKI}" --ca-name="${ZITI_CONTROLLER_INTERMEDIATE_NAME}" \
        --client-file="${ZITI_NETWORK}-dotzeet" \
        --client-name "${ZITI_NETWORK} Management"
else
  echo "Creating ziti-fabric client certificate for network: ${ZITI_NETWORK}"
  echo "key exists"
fi
echo " "

pki_client_server "${ZITI_CONTROLLER_HOSTNAME}" "${ZITI_CONTROLLER_INTERMEDIATE_NAME}" "${ZITI_CONTROLLER_IP_OVERRIDE}"
pki_client_server "${ZITI_EDGE_CONTROLLER_HOSTNAME}" "${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}" "${ZITI_EDGE_CONTROLLER_IP_OVERRIDE}"
#pki_client_server "${ZITI_EDGE_ROUTER_HOSTNAME}" "${ZITI_CONTROLLER_INTERMEDIATE_NAME}" "${ZITI_EDGE_ROUTER_IP_OVERRIDE}"
