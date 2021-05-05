cat > ${ZITI_HOME}/identities.yml <<IdentitiesJsonHereDoc
---
default:
  caCert:   "${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_CONTROLLER_HOSTNAME}-server.chain.pem"
  cert:     "${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_NETWORK}-dotzeet.cert"
  key:      "${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/keys/${ZITI_NETWORK}-dotzeet.key"
  endpoint: tls:${ZITI_CONTROLLER_HOSTNAME}:${ZITI_FAB_MGMT_PORT}
IdentitiesJsonHereDoc
