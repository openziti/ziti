#!/bin/bash
. ziti.env

router_name=$(hostname)
echo "Generating a key, a server certificate and client certificate for ${router_name}"
echo ".......... Generating key for ${router_name}"
ziti pki create key --pki-root="${ZITI_PKI}" --ca-name="${intermediate_name}" --key-file="${router_name}"
echo ".......... Generating server certificate for ${router_name}"
ziti pki create server --pki-root="${ZITI_PKI}" --ca-name="${intermediate_name}" --key-file="${router_name}" --server-file="${router_name}-server" --dns "localhost" --dns "${router_name}" --ip "127.0.0.1"
echo ".......... Generating client certificate for ${router_name}"
ziti pki create client --pki-root="${ZITI_PKI}" --ca-name="${intermediate_name}" --key-file="${router_name}" --client-file="${router_name}-client" --client-name="${router_name}"
echo "Creating a file with the root CA, intermediate and the identity cert - some processes require the full chain to be supplied"
cat "${ZITI_PKI}/${intermediate_name}/certs/${router_name}-server.chain.pem" "${ZITI_PKI}/${ca_name}/certs/${ca_name}.cert" > "${ZITI_PKI}/${intermediate_name}/certs/${router_name}-full-chain.pem"

cat > $ZITI_HOME/${router_name}.yml <<RouterConfigHereDoc
v: 2

identity:
  ca: "${ZITI_PKI}/${intermediate_name}/certs/${intermediate_name}-full-chain.pem"
  key: "${ZITI_PKI}/${intermediate_name}/keys/${router_name}.key"
  cert: "${ZITI_PKI}/${intermediate_name}/certs/${router_name}-client.cert"
  server_cert: "${ZITI_PKI}/${intermediate_name}/certs/${router_name}-full-chain.pem"

trace:
  path: "$ZITI_HOME/${router_name}.trace"

ctrl:
  endpoint: "tls:${fabric_controller_name}:6262"

link:
  listener: "tls:0.0.0.0:6000"
  advertise: "tls:${router_name}:6000"

listeners:
  - binding: transport
    address: tls:0.0.0.0:7000
    options:
      retransmission: true
      randomDrops: false
      drop1InN: 500

RouterConfigHereDoc

# register the router and start it...
echo "registering router ${router_name} with fabric controller at ${fabric_controller_uri}"
ziti-fabric create router -e "${fabric_controller_uri}" "${ZITI_PKI}/${intermediate_name}/certs/${router_name}-client.cert"

echo "starting router ${router_name}:"
echo "ziti-router run $ZITI_HOME/${router_name}.yml > $ZITI_HOME/${router_name}.log 2>&1 &"
ziti-router run $ZITI_HOME/${router_name}.yml 
