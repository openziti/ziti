#!/bin/bash
cat > ${ZITI_HOME}/${ZITI_EDGE_ROUTER_HOSTNAME}.yaml <<HereDocForEdgeRouter
v: 3

identity:
  cert:                 ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_EDGE_ROUTER_HOSTNAME}-router-client.cert
  server_cert:          ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_EDGE_ROUTER_HOSTNAME}-router-server.cert
  key:                  ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/keys/${ZITI_EDGE_ROUTER_HOSTNAME}-router-server.key
  ca:                   ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_EDGE_ROUTER_HOSTNAME}-router-server.chain.pem

ctrl:
  endpoint:             tls:${ZITI_CONTROLLER_HOSTNAME}:${ZITI_FAB_CTRL_PORT}

link:
  listeners:
    - binding:          transport
      bind:             tls:0.0.0.0:10080
      advertise:        tls:${ZITI_EDGE_ROUTER_HOSTNAME}:10080
      options:
        outQueueSize:   16
  dialers:
    - binding: transport

listeners:
  - binding: tunnel
    options:
      mode: host #tproxy|tun|host
  - binding: edge
    address: ws:0.0.0.0:3023
    options:
      advertise: ${ZITI_EDGE_ROUTER_HOSTNAME}:3023
      connectTimeoutMs: 5000
      getSessionTimeout: 60s

edge:
  csr:
    country: US
    province: NC
    locality: Charlotte
    organization: NetFoundry
    organizationalUnit: Ziti
    sans:
      dns:
        - ${ZITI_EDGE_ROUTER_HOSTNAME}
        - localhost
      ip:
        - "127.0.0.1"

transport:
  ws:
    writeTimeout:      10
    readTimeout:       5
    idleTimeout:       5
    pongTimeout:       60
    pingInterval:      54
    handshakeTimeout:  10
    readBufferSize:    4096
    writeBufferSize:   4096
    enableCompression: true
    server_cert:       ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_EDGE_ROUTER_HOSTNAME}-router-server.cert
    key:               ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/keys/${ZITI_EDGE_ROUTER_HOSTNAME}-router-server.key

forwarder:
  latencyProbeInterval: 1000
  xgressDialQueueLength: 1000
  xgressDialWorkerCount: 128
  linkDialQueueLength: 1000
  linkDialWorkerCount: 10
HereDocForEdgeRouter
