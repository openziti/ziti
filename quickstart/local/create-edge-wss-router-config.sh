#!/bin/bash
cat > ${ZITI_HOME}/${ZITI_EDGE_WSS_ROUTER_NAME}.yaml <<HereDocForEdgeRouter
v: 3

identity:
  cert:                 ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_EDGE_WSS_ROUTER_HOSTNAME}-router-client.cert
  server_cert:          ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_EDGE_WSS_ROUTER_HOSTNAME}-router-server.cert
  key:                  ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/keys/${ZITI_EDGE_WSS_ROUTER_HOSTNAME}-router-server.key
  ca:                   ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_EDGE_WSS_ROUTER_HOSTNAME}-router-server.chain.pem

ctrl:
  endpoint:             tls:${ZITI_CONTROLLER_HOSTNAME}:${ZITI_FAB_CTRL_PORT}

listeners:
  - binding: edge_wss
    address: tls:0.0.0.0:3023
    identity:
      server_cert: ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_EDGE_WSS_ROUTER_HOSTNAME}-router-server.cert
      key:  ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/keys/${ZITI_EDGE_WSS_ROUTER_HOSTNAME}-router-server.key
    options:
      advertise: ${ZITI_EDGE_WSS_ROUTER_HOSTNAME}:3023

edge:
  underlay_type: wss
  csr:
    country: US
    province: NC
    locality: Charlotte
    organization: NetFoundry
    organizationalUnit: Ziti
    sans:
      dns:
        - ${ZITI_EDGE_WSS_ROUTER_HOSTNAME}
      ip:
        - "127.0.0.1"

dialers:
  - binding: udp
  - binding: transport
HereDocForEdgeRouter
