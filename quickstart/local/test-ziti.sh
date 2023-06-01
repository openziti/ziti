ziti edge controller login "${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS}" -u "${ZITI_USER}" -p "${ZITI_PWD}" -y

ziti edge delete service netcatsvc
ziti edge delete service zcatsvc
ziti edge controller delete service httpbinsvc
ziti edge controller delete service iperfsvc

ziti edge controller delete config netcatconfig
ziti edge controller delete config zcatconfig
ziti edge controller delete config httpbinsvcconfig
ziti edge controller delete config iperfsvcconfig

ziti edge controller create config httpbinsvcconfig ziti-tunneler-client.v1 '{ "hostname" : "httpbin.ziti", "port" : 8000 }'
ziti edge controller create service httpbinsvc --configs httpbinsvcconfig
ziti edge controller create terminator httpbinsvc "${ZITI_ROUTER_ADVERTISED_ADDRESS}" tcp:localhost:80

ziti edge controller create config netcatconfig ziti-tunneler-client.v1 '{ "hostname" : "localhost", "port" : 7256 }'
ziti edge controller create service netcatsvc --configs netcatconfig
ziti edge controller create terminator netcatsvc "${ZITI_ROUTER_ADVERTISED_ADDRESS}" tcp:localhost:7256

ziti edge controller create config zcatconfig ziti-tunneler-client.v1 '{ "hostname" : "zcat.ziti", "port" : 7256 }'
ziti edge controller create service zcatsvc --configs zcatconfig
ziti edge controller create terminator zcatsvc "${ZITI_ROUTER_ADVERTISED_ADDRESS}" tcp:localhost:7256

ziti edge controller create config iperfsvcconfig ziti-tunneler-client.v1 '{ "hostname" : "iperf3.ziti", "port" : 15000 }'
ziti edge controller create service iperfsvc --configs iperfsvcconfig 
ziti edge controller create terminator iperfsvc "${ZITI_ROUTER_ADVERTISED_ADDRESS}" tcp:localhost:5201

ziti edge controller delete identity "test_identity"
ziti edge controller create identity device "test_identity" -o "${ZITI_HOME}/test_identity".jwt

ziti edge controller delete service-policy dial-all
ziti edge controller create service-policy dial-all Dial --service-roles '#all' --identity-roles '#all'

#ziti-enroller --jwt "${ZITI_HOME}/test_identity.jwt" -o "${ZITI_HOME}/test_identity".json

#ziti-tunnel proxy netcatsvc:8145 -i "${ZITI_HOME}/test_identity".json > "${ZITI_HOME}/ziti-test_identity.log" 2>&1 &
cp "${ZITI_HOME}/test_identity.jwt" /mnt/v/temp/ziti-windows-tunneler/_new_id.jwt

