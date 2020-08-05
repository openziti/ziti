suffix=$(date +"%b-%d-%H%M")
idname="User${suffix}"

ziti edge controller login "${ZITI_EDGE_API_HOSTNAME}" -u "${ZITI_USER}" -p "${ZITI_PWD}" -c "${ZITI_PKI}/${ZITI_EDGE_ROOTCA_NAME}/certs/${ZITI_EDGE_INTERMEDIATE_NAME}.cert"

ziti edge controller delete identity "${idname}"
ziti edge controller create identity device "${idname}" -o "${ZITI_HOME}/test_identity".jwt

cp "${ZITI_HOME}/test_identity".jwt /mnt/v/temp/ziti-windows-tunneler/_new_id.jwt

