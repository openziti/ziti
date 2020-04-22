ziti edge controller login "${ZITI_EDGE_API_HOSTNAME}" -u "${ZITI_USER}" -p "${ZITI_PWD}" -c "${ZITI_PKI}/${ZITI_EDGE_ROOTCA_NAME}/certs/${ZITI_EDGE_INTERMEDIATE_NAME}.cert"

ziti edge controller delete service netcatsvc
ziti edge controller delete service zcatsvc

ziti edge controller delete config netcatconfig
ziti edge controller delete config zcatsvcconfig

ziti edge controller create config netcatconfig ziti-tunneler-client.v1 '{ "hostname" : "localhost", "port" : 7256 }'
ziti edge controller create service netcatsvc --configs netcatconfig
ziti edge controller create terminator netcatsvc "${ZITI_EDGE_ROUTER_NAME}" tcp:localhost:7256

ziti edge controller create config zcatconfig ziti-tunneler-client.v1 '{ "hostname" : "zcat", "port" : 7256 }'
ziti edge controller create service zcatsvc --configs zcatconfig
ziti edge controller create terminator zcatsvc "${ZITI_EDGE_ROUTER_NAME}" tcp:localhost:7256

ziti edge controller delete identity "test_identity"
ziti edge controller create identity device "test_identity" -o "${ZITI_HOME}/test_identity".jwt

ziti edge controller create service-policy dial-all Dial --service-roles '#all' --identity-roles '#all'

ziti-enroller --jwt "${ZITI_HOME}/test_identity".jwt -o "${ZITI_HOME}/test_identity".json

ziti-tunnel proxy netcat7256:8145 -i "${ZITI_HOME}/test_identity".json > "${ZITI_HOME}/ziti-test_identity.log" 2>&1 &
