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

pki_client_server "${ZITI_EDGE_ROUTER_HOSTNAME}" "${ZITI_CONTROLLER_INTERMEDIATE_NAME}" "${ZITI_EDGE_ROUTER_IP_OVERRIDE}"
