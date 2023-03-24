ziti edge login "${ZITI_EDGE_CONTROLLER_API}" -u "${ZITI_USER}" -p "${ZITI_PWD}" -c "${ZITI_PKI}/${ZITI_EDGE_CONTROLLER_ROOTCA_NAME}/certs/${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}.cert"

ziti edge delete service zcatsvc
ziti edge delete config zcatconfig

ziti edge create config zcatconfig ziti-tunneler-client.v1 '{ "hostname" : "zcat.ziti", "port" : 7256 }'
ziti edge create service zcatsvc --configs zcatconfig
ziti edge create terminator zcatsvc "${ZITI_ROUTER_ADVERTISED_ADDRESS}" tcp:localhost:7256


