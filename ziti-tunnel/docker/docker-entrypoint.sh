#!/usr/bin/env bash

set -euo pipefail

function alldone() {
    # if successfully sent to background then send SIGINT to trigger a cleanup
    # of iptables mangle rules and loopback assignments
    [[ "${ZITI_TUNNEL_PID:-}" =~ ^[0-9]+$ ]] && {
        kill -INT "$ZITI_TUNNEL_PID"
        # let entrypoint script exit after ziti-tunnel PID
        wait "$ZITI_TUNNEL_PID"
    }
}
trap alldone exit

# Ensure that ziti-tunnel's identity is stored on a volume
# so we don't throw away the one-time enrollment token

persisted_dir="/netfoundry"
wc_lines=$(df / "${persisted_dir}" 2> /dev/null | uniq | wc -l)
if [ "${wc_lines}" != "3" ]; then
    echo "ERROR: please run this image with ${persisted_dir} mounted on a volume"
    exit 1
fi

# try to figure out the client name if it wasn't provided
if [ -z "${NF_REG_NAME}" ]; then
    if [ -n "${IOTEDGE_DEVICEID}" ]; then
        echo "setting NF_REG_NAME to \${IOTEDGE_DEVICEID} (${IOTEDGE_DEVICEID})"
        NF_REG_NAME="${IOTEDGE_DEVICEID}"
    fi
fi
if [ -z "${NF_REG_NAME}" ]; then
    echo "ERROR: please set the NF_REG_NAME environment variable when running this image"
    exit 1
fi

json="${persisted_dir}/${NF_REG_NAME}.json"
if [ ! -f "${json}" ]; then
    echo "WARN: identity configuration ${json} does not exist" >&2
    for dir in  "/var/run/secrets/kubernetes.io/enrollment-token" \
                "/enrollment-token" \
                "${persisted_dir}";
        do
            [[ -d ${dir} ]] || {
                echo "INFO: ${dir} is not a directory"
                continue
            }
            _jwt="${dir}/${NF_REG_NAME}.jwt"
            echo "INFO: looking for ${_jwt}"
            if [ -f "${_jwt}" ]; then
                jwt="${_jwt}"
                break
            else
                echo "WARN: failed to find ${_jwt} in ${dir}" >&2
            fi
    done
    if [ -z "${jwt:-}" ]; then
        echo "ERROR: ${NF_REG_NAME}.jwt was not found in the expected locations" >&2
        exit 1
    fi
    echo "INFO: enrolling with token from file '${jwt}' and value '$(< ${jwt})'"
    ziti-tunnel enroll --jwt "${jwt}" --out "${json}"
fi

echo "INFO: probing iptables"
if iptables -t mangle -S --wait 2>&1 | grep -q "iptables-legacy tables present"; then
    for LEGACY in {ip{,6},eb,arp}tables; do
        if which ${LEGACY}-legacy &>/dev/null; then
            echo "INFO: updating $LEGACY alternative to ${LEGACY}-legacy"
            update-alternatives --set $LEGACY "$(which ${LEGACY}-legacy)"
        else
            echo "WARN: not updating $LEGACY alternative to ${LEGACY}-legacy" >&2
        fi
    done
fi

# optionally run an alternative shell CMD
echo "running ziti-tunnel"
set -x
ziti-tunnel -i "${json}" "${@}" &
ZITI_TUNNEL_PID=$!
wait $ZITI_TUNNEL_PID
