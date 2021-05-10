#!/bin/bash

source ./pki-functions.sh

pki_client_server "${ZITI_EDGE_ROUTER_HOSTNAME}" "${ZITI_CONTROLLER_INTERMEDIATE_NAME}" "${ZITI_EDGE_ROUTER_IP_OVERRIDE}"
