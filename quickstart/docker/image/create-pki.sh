#!/bin/bash
function pki_client_server {
  name_local=$1
  ZITI_CA_NAME_local=$2

  if ! test -f "${ZITI_PKI}/${ZITI_CA_NAME_local}/keys/${name_local}-server.key"; then
    echo "Creating server cert from ca: ${ZITI_CA_NAME_local} for ${name_local}"
    ziti pki create server --pki-root="${ZITI_PKI}" --ca-name "${ZITI_CA_NAME_local}" \
          --server-file "${name_local}-server" \
          --dns "${name_local},localhost" --ip 127.0.0.1 \
          --server-name "${name_local} server certificate"
  else
    echo "Creating server cert from ca: ${ZITI_CA_NAME_local} for ${name_local}"
    echo "key exists"
  fi

  if ! test -f "${ZITI_PKI}/${ZITI_CA_NAME_local}/keys/${name_local}-client.key"; then
    echo "Creating client cert from ca: ${ZITI_CA_NAME_local} for ${name_local}"
    ziti pki create client --pki-root="${ZITI_PKI}" --ca-name "${ZITI_CA_NAME_local}" \
          --client-file "${name_local}-client" \
          --key-file "${name_local}-server" \
          --client-name "${name_local}"
  else
    echo "Creating client cert from ca: ${ZITI_CA_NAME_local} for ${name_local}"
    echo "key exists"
  fi
  echo " "
}

function pki_create_ca {
  if ! test -f "${ZITI_PKI}/${1}/keys/${1}.key"; then
    echo "Creating CA: ${1}"
    ziti pki create ca --pki-root="${ZITI_PKI}" --ca-file="${1}" --ca-name="${1} Root CA"
  else
    echo "Creating CA: ${1}"
    echo "key exists"
  fi
  echo " "
}

function pki_create_intermediate {
  if ! test -f "${ZITI_PKI}/${2}/keys/${2}.key"; then
    echo "Creating intermediate: ${1} ${2} ${3}"
    ziti pki create intermediate --pki-root "${ZITI_PKI}" --ca-name "${1}" \
          --intermediate-name "${2}" \
          --intermediate-file "${2}" --max-path-len ${3}
  else
    echo "Creating intermediate: ${1} ${2} ${3}"
    echo "key exists"
  fi
  echo " "
}

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

pki_client_server "${ZITI_CONTROLLER_HOSTNAME}" "${ZITI_CONTROLLER_INTERMEDIATE_NAME}"
pki_client_server "${ZITI_EDGE_CONTROLLER_HOSTNAME}" "${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}"
pki_client_server "${ZITI_EDGE_ROUTER_HOSTNAME}" "${ZITI_CONTROLLER_INTERMEDIATE_NAME}"
pki_client_server "${ZITI_EDGE_WSS_ROUTER_NAME}" "${ZITI_CONTROLLER_INTERMEDIATE_NAME}"
pki_client_server "${ZITI_ROUTER_BR_HOSTNAME}" "${ZITI_CONTROLLER_INTERMEDIATE_NAME}"
pki_client_server "${ZITI_ROUTER_BLUE_HOSTNAME}" "${ZITI_CONTROLLER_INTERMEDIATE_NAME}"
pki_client_server "${ZITI_ROUTER_RED_HOSTNAME}" "${ZITI_CONTROLLER_INTERMEDIATE_NAME}"
pki_client_server "${ZITI_ZAC_HOSTNAME}" "${ZITI_CONTROLLER_INTERMEDIATE_NAME}"
