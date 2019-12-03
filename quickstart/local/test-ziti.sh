ziti edge controller create service netcat7256 localhost 7256 "${ZITI_ROUTER_BR_HOSTNAME}" tcp://localhost:7256

ziti edge controller create identity device "test_identity" -o "${ZITI_HOME}/test_identity".jwt

ziti edge controller create app-wan appwan01 -s netcat7256 -i "test_identity"

ziti-enroller --jwt "${ZITI_HOME}/test_identity".jwt -o "${ZITI_HOME}/test_identity".json

ziti-tunnel proxy netcat7256:8145 -i "${ZITI_HOME}/test_identity".json > "${ZITI_HOME}/ziti-test_identity.log" 2>&1 &
