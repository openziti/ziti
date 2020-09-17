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
  - binding: edge
    address: wss:0.0.0.0:3023
    options:
      advertise: ${ZITI_EDGE_WSS_ROUTER_HOSTNAME}:3023

edge:
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

transport:
  wss:
    writeTimeout:      10
    readTimeout:       5
    idleTimeout:       5
    pongTimeout:       60
    pingInterval:      54
    handshakeTimeout:  10
    readBufferSize:    4096
    writeBufferSize:   4096
    enableCompression: true
    server_cert:       ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_EDGE_WSS_ROUTER_HOSTNAME}-router-server.cert
    key:               ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/keys/${ZITI_EDGE_WSS_ROUTER_HOSTNAME}-router-server.key

HereDocForEdgeRouter
