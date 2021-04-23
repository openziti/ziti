#!/bin/bash
function pki_client_server {
    name_local=$1
    ZITI_CA_NAME_local=$2

    echo "Creating server and client certs from ca: ${ZITI_CA_NAME_local} for ${name_local}"

    ziti pki create server --pki-root="${ZITI_PKI}" --ca-name "${ZITI_CA_NAME_local}" \
    --server-file "${name_local}-server" \
    --dns "${name_local}, localhost" --ip 127.0.0.1 \
    --server-name "${name_local} server certificate"

    ziti pki create client --pki-root="${ZITI_PKI}" --ca-name "${ZITI_CA_NAME_local}" \
    --client-file "${name_local}-client" \
    --key-file "${name_local}-server" \
    --client-name "${name_local}"
    echo ""
}

echo "Creating CONTROLLER CA: ${ZITI_CONTROLLER_ROOTCA_NAME}"
ziti pki create ca --pki-root="${ZITI_PKI}" --ca-file="${ZITI_CONTROLLER_ROOTCA_NAME}" --ca-name="${ZITI_CONTROLLER_ROOTCA_NAME} Root CA"
echo ""

echo "Creating controller intermediate: ${ZITI_CONTROLLER_INTERMEDIATE_NAME}"
ziti pki create intermediate --pki-root "${ZITI_PKI}" --ca-name "${ZITI_CONTROLLER_ROOTCA_NAME}" \
    --intermediate-name "${ZITI_CONTROLLER_INTERMEDIATE_NAME}" \
    --intermediate-file "${ZITI_CONTROLLER_INTERMEDIATE_NAME}" --max-path-len 1
echo ""

echo "Creating ziti-fabric client certificate: ${ZITI_NETWORK}-dotzeet"
ziti pki create client --pki-root="${ZITI_PKI}" --ca-name="${ZITI_CONTROLLER_INTERMEDIATE_NAME}" \
--client-file="${ZITI_NETWORK}-dotzeet" \
--client-name "${ZITI_NETWORK} Management"
echo

echo "==============================================================="
echo "=== creating edge related  CA chain                         ==="
echo "==============================================================="
echo "Creating EDGE CA: ${ZITI_EDGE_CONTROLLER_ROOTCA_NAME}"
ziti pki create ca --pki-root="${ZITI_PKI}" --ca-file="${ZITI_EDGE_CONTROLLER_ROOTCA_NAME}" --ca-name="${ZITI_EDGE_CONTROLLER_ROOTCA_NAME} Root CA"
echo ""

echo "Creating edge intermediate: ${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}"
ziti pki create intermediate --pki-root "${ZITI_PKI}" --ca-name "${ZITI_EDGE_CONTROLLER_ROOTCA_NAME}" \
    --intermediate-name "${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}" \
    --intermediate-file "${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}" --max-path-len 1
echo ""

echo "==============================================================="
echo "=== creating signing related CA chain                       ==="
echo "==============================================================="
echo "Creating SIGNING CA: ${ZITI_SIGNING_ROOTCA_NAME}"
ziti pki create ca --pki-root="${ZITI_PKI}" --ca-file="${ZITI_SIGNING_ROOTCA_NAME}" --ca-name="${ZITI_SIGNING_ROOTCA_NAME} Root CA"
echo ""

echo "Creating signing intermediate, intermediate (2 intermediates): ${ZITI_SIGNING_SPURIOUS_NAME}"
ziti pki create intermediate --pki-root "${ZITI_PKI}" --ca-name "${ZITI_SIGNING_ROOTCA_NAME}" \
    --intermediate-name "${ZITI_SIGNING_SPURIOUS_NAME}" \
    --intermediate-file "${ZITI_SIGNING_SPURIOUS_NAME}" --max-path-len 2
echo ""

echo "Creating signing intermediate: ${ZITI_SIGNING_INTERMEDIATE_NAME}"
ziti pki create intermediate --pki-root "${ZITI_PKI}" --ca-name "${ZITI_SIGNING_SPURIOUS_NAME}" \
    --intermediate-name "${ZITI_SIGNING_INTERMEDIATE_NAME}" \
    --intermediate-file "${ZITI_SIGNING_INTERMEDIATE_NAME}" --max-path-len 1
echo ""

pki_client_server "${ZITI_CONTROLLER_HOSTNAME}" "${ZITI_CONTROLLER_INTERMEDIATE_NAME}"
pki_client_server "${ZITI_EDGE_CONTROLLER_HOSTNAME}" "${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}"
pki_client_server "${ZITI_ZAC_HOSTNAME}" "${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}"
#pki_client_server "${ZITI_EDGE_CONTROLLER_HOSTNAME}" "${ZITI_CONTROLLER_INTERMEDIATE_NAME}"
#pki_client_server "${ZITI_EDGE_CONTROLLER_HOSTNAME}" "${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}"
#pki_client_server "${ZITI_EDGE_ROUTER_HOSTNAME}" "${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}"
#pki_client_server "${ZITI_EDGE_WSS_ROUTER_NAME}" "${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}"

