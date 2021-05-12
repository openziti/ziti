#!/bin/bash

source ${ZITI_SCRIPTS}/pki-functions.sh

pki_client_server "${ZITI_EDGE_ROUTER_RAWNAME}" "${ZITI_CONTROLLER_INTERMEDIATE_NAME}" "${ZITI_EDGE_ROUTER_IP_OVERRIDE}"
