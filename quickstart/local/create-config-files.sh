#!/bin/bash
${curdir}/create-controller-config.sh
${curdir}/create-edge-router-config.sh
${curdir}/create-edge-wss-router-config.sh


echo "
---
default:
  caCert:   ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_CONTROLLER_HOSTNAME}-server.chain.pem
  cert:     ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_NETWORK}-dotzeet.cert
  key:      ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/keys/${ZITI_NETWORK}-dotzeet.key
  endpoint: tls:${ZITI_CONTROLLER_HOSTNAME}:${ZITI_FAB_MGMT_PORT}
" > $ZITI_HOME/identities.yml


echo "v: 3

identity:
  cert:                 ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_ROUTER_BR_HOSTNAME}-client.cert
  server_cert:          ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_ROUTER_BR_HOSTNAME}-server.cert
  key:                  ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/keys/${ZITI_ROUTER_BR_HOSTNAME}-server.key
  ca:                   ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_ROUTER_BR_HOSTNAME}-server.chain.pem

ctrl:
  endpoint:             tls:${ZITI_CONTROLLER_HOSTNAME}:${ZITI_FAB_CTRL_PORT}

link:
  listener:             tls:${ZITI_ROUTER_BR_HOSTNAME}:6001
" > $ZITI_HOME/${ZITI_ROUTER_BR_NAME}.yaml

echo "v: 3

identity:
  cert:                 ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_ROUTER_BLUE_HOSTNAME}-client.cert
  server_cert:          ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_ROUTER_BLUE_HOSTNAME}-server.cert
  key:                  ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/keys/${ZITI_ROUTER_BLUE_HOSTNAME}-server.key
  ca:                   ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_ROUTER_BLUE_HOSTNAME}-server.chain.pem

ctrl:
  endpoint:             tls:${ZITI_CONTROLLER_HOSTNAME}:${ZITI_FAB_CTRL_PORT}
" > $ZITI_HOME/${ZITI_ROUTER_BLUE_NAME}.yaml


echo "v: 3

identity:
  cert:                 ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_ROUTER_RED_HOSTNAME}-client.cert
  server_cert:          ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_ROUTER_RED_HOSTNAME}-server.cert
  key:                  ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/keys/${ZITI_ROUTER_RED_HOSTNAME}-server.key
  ca:                   ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_ROUTER_RED_HOSTNAME}-server.chain.pem

ctrl:
  endpoint:             tls:${ZITI_CONTROLLER_HOSTNAME}:${ZITI_FAB_CTRL_PORT}

listeners:
  # example xgress_transport
  - binding:            transport
    address:            tls:0.0.0.0:7002
    options:
      retransmission:   true

dialers:
  - binding:            transport
    options:
      retransmission:   true

" > $ZITI_HOME/${ZITI_ROUTER_RED_NAME}.yaml
