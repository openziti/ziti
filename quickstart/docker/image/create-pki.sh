#!/bin/bash
function pki_client_server {
    name_local=$1
    ZITI_CA_NAME_local=$2

    echo "Creating server and client certs from ca: ${ZITI_CA_NAME_local} for ${name_local}"

    ziti pki create server --pki-root="${ZITI_PKI}" --ca-name "${ZITI_CA_NAME_local}" \
    --server-file "${name_local}-server" \
    --dns "${name_local}" --ip 127.0.0.1 \
    --server-name "${name_local} server certificate"

    ziti pki create client --pki-root="${ZITI_PKI}" --ca-name "${ZITI_CA_NAME_local}" \
    --client-file "${name_local}-client" \
    --key-file "${name_local}-server" \
    --client-name "${name_local}"
    echo "--- DONE"
    echo ""
}

echo "Creating CONTROLLER CA: ${ZITI_CONTROLLER_ROOTCA_NAME}"
ziti pki create ca --pki-root="${ZITI_PKI}" --ca-file="${ZITI_CONTROLLER_ROOTCA_NAME}" --ca-name="${ZITI_CONTROLLER_ROOTCA_NAME} Root CA"
echo "--- DONE"
echo ""

echo "Creating EDGE CA: ${ZITI_EDGE_ROOTCA_NAME}"
ziti pki create ca --pki-root="${ZITI_PKI}" --ca-file="${ZITI_EDGE_ROOTCA_NAME}" --ca-name="${ZITI_EDGE_ROOTCA_NAME} Root CA"
echo "--- DONE"
echo ""

echo "Creating SIGNING CA: ${ZITI_SIGNING_ROOTCA_NAME}"
ziti pki create ca --pki-root="${ZITI_PKI}" --ca-file="${ZITI_SIGNING_ROOTCA_NAME}" --ca-name="${ZITI_SIGNING_ROOTCA_NAME} Root CA"
echo "--- DONE"
echo ""

echo "Creating controller intermediate"
ziti pki create intermediate --pki-root "${ZITI_PKI}" --ca-name "${ZITI_CONTROLLER_ROOTCA_NAME}" \
    --intermediate-name "${ZITI_CONTROLLER_INTERMEDIATE_NAME}" \
    --intermediate-file "${ZITI_CONTROLLER_INTERMEDIATE_NAME}" --max-path-len 1
echo "--- DONE"
echo ""

echo "Creating edge intermediate"
ziti pki create intermediate --pki-root "${ZITI_PKI}" --ca-name "${ZITI_EDGE_ROOTCA_NAME}" \
    --intermediate-name "${ZITI_EDGE_INTERMEDIATE_NAME}" \
    --intermediate-file "${ZITI_EDGE_INTERMEDIATE_NAME}" --max-path-len 1
echo "--- DONE"
echo ""

echo "Creating signing intermediate, intermediate!"
ziti pki create intermediate --pki-root "${ZITI_PKI}" --ca-name "${ZITI_SIGNING_ROOTCA_NAME}" \
    --intermediate-name "${ZITI_SIGNING_INTERMEDIATE_NAME}_spurious_intermediate" \
    --intermediate-file "${ZITI_SIGNING_INTERMEDIATE_NAME}_spurious_intermediate" --max-path-len 2
echo "--- DONE"
echo ""

echo "Creating signing intermediate"
ziti pki create intermediate --pki-root "${ZITI_PKI}" --ca-name "${ZITI_SIGNING_INTERMEDIATE_NAME}_spurious_intermediate" \
    --intermediate-name "${ZITI_SIGNING_INTERMEDIATE_NAME}" \
    --intermediate-file "${ZITI_SIGNING_INTERMEDIATE_NAME}" --max-path-len 1
echo "--- DONE"
echo ""

echo "Creating ziti-fabric client certificate"
ziti pki create client --pki-root="${ZITI_PKI}" --ca-name="${ZITI_CONTROLLER_INTERMEDIATE_NAME}" \
--client-file="${ZITI_NETWORK}-dotzeet" \
--client-name "${ZITI_NETWORK} Management"
echo "--- DONE"
echo ""

pki_client_server "${ZITI_EDGE_HOSTNAME}" "${ZITI_EDGE_INTERMEDIATE_NAME}"
pki_client_server "${ZITI_CONTROLLER_HOSTNAME}" "${ZITI_CONTROLLER_INTERMEDIATE_NAME}"
pki_client_server "${ZITI_ROUTER_BR_HOSTNAME}" "${ZITI_CONTROLLER_INTERMEDIATE_NAME}"
pki_client_server "${ZITI_ROUTER_BLUE_HOSTNAME}" "${ZITI_CONTROLLER_INTERMEDIATE_NAME}"
pki_client_server "${ZITI_ROUTER_RED_HOSTNAME}" "${ZITI_CONTROLLER_INTERMEDIATE_NAME}"
#pki_client_server "${ZITI_EDGE_ROUTER_HOSTNAME}" "${ZITI_EDGE_INTERMEDIATE_NAME}"
