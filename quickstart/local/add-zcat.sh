ziti edge login "${ZITI_EDGE_CONTROLLER_API}" -u "${ZITI_USER}" -p "${ZITI_PWD}" -y

ziti edge delete service zcatsvc
ziti edge delete config zcatconfig

ziti edge create config zcatconfig ziti-tunneler-client.v1 '{ "hostname" : "zcat.ziti", "port" : 7256 }'
ziti edge create service zcatsvc --configs zcatconfig
ziti edge create terminator zcatsvc "${ZITI_EDGE_ROUTER_HOSTNAME}" tcp:localhost:7256


