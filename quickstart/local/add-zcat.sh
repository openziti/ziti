ziti edge controller login "${ZITI_EDGE_API_HOSTNAME}" -u "${ZITI_USER}" -p "${ZITI_PWD}" -c "${ZITI_PKI}/${ZITI_EDGE_ROOTCA_NAME}/certs/${ZITI_EDGE_INTERMEDIATE_NAME}.cert"

ziti edge controller delete service zcatsvc
ziti edge controller delete config zcatconfig

ziti edge controller create config zcatconfig ziti-tunneler-client.v1 '{ "hostname" : "zcat.ziti", "port" : 7256 }'
ziti edge controller create service zcatsvc --configs zcatconfig
ziti edge controller create terminator zcatsvc "${ZITI_EDGE_ROUTER_HOSTNAME}" tcp:localhost:7256


