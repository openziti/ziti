suffix=$(date +"%b-%d-%H%M")
idname="User${suffix}"

ziti edge login "${ZITI_EDGE_CONTROLLER_API}" -u "${ZITI_USER}" -p "${ZITI_PWD}" -y

ziti edge delete identity "${idname}"
ziti edge create identity device "${idname}" -o "${ZITI_HOME}/test_identity".jwt

cp "${ZITI_HOME}/test_identity".jwt /mnt/v/temp/ziti-windows-tunneler/_new_id.jwt

