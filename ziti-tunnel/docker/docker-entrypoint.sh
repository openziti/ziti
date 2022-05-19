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

IDENTITIES_DIR="/netfoundry"
if ! mountpoint "${IDENTITIES_DIR}" &>/dev/null; then
    echo "ERROR: please run this container with a volume mounted on ${IDENTITIES_DIR}" >&2
    exit 1
fi

# IOTEDGE_DEVICEID is a standard var assigned by Azure IoT
if [[ -z "${NF_REG_NAME:-}" ]]; then
    if [[ -n "${IOTEDGE_DEVICEID:-}" ]]; then
        echo "DEBUG: setting NF_REG_NAME to \${IOTEDGE_DEVICEID} (${IOTEDGE_DEVICEID})"
        NF_REG_NAME="${IOTEDGE_DEVICEID}"
    fi
fi

typeset -a TUNNEL_OPTS
# if identity file name, else critical error
if [[ -n "${NF_REG_NAME:-}" ]]; then
    IDENTITY_FILE="${IDENTITIES_DIR}/${NF_REG_NAME}.json"
    TUNNEL_OPTS=("--identity" "${IDENTITY_FILE}")
    # if non-empty identity file
    if [[ -s "${IDENTITY_FILE}" ]]; then
        echo "DEBUG: found identity file ${IDENTITY_FILE}"
    # look for enrollment token
    else
        echo "DEBUG: identity file ${IDENTITY_FILE} does not exist"
        for dir in  "/var/run/secrets/netfoundry.io/enrollment-token" \
                    "/enrollment-token" \
                    "${IDENTITIES_DIR}"; do
            JWT_CANDIDATE="${dir}/${NF_REG_NAME}.jwt"
            echo "DEBUG: looking for ${JWT_CANDIDATE}"
            if [[ -s "${JWT_CANDIDATE}" ]]; then
                JWT_FILE="${JWT_CANDIDATE}"
                break
            fi
        done
        if [[ -n "${JWT_FILE:-}" ]]; then
            echo "DEBUG: enrolling ${JWT_FILE}"
            ziti-tunnel enroll --verbose --jwt "${JWT_FILE}" --identity "${IDENTITY_FILE}" || {
                echo "ERROR: failed to enroll with token from ${JWT_FILE} ($(wc -c < "${JWT_FILE}")B)" >&2
                exit 1
            }
        elif [[ -n "${NF_REG_TOKEN:-}" ]]; then
            echo "DEBUG: attempting enrollment with NF_REG_TOKEN"
            JWT_FILE=${IDENTITIES_DIR}/${NF_REG_NAME}.jwt
            echo "${NF_REG_TOKEN}" >| ${JWT_FILE}
            ziti-tunnel enroll --verbose --jwt ${JWT_FILE} --identity "${IDENTITY_FILE}" || {
                echo "ERROR: failed to enroll with token from NF_REG_TOKEN ($(wc -c <<<"${NF_REG_TOKEN}")B)" >&2
                exit 1
            }
        elif ! [[ -t 0 ]]; then
            echo "DEBUG: trying to get token from stdin" >&2
            JWT_FILE=${IDENTITIES_DIR}/${NF_REG_NAME}.jwt
            while read -r; do
                echo "$REPLY" >> ${JWT_FILE}
            done
            ziti-tunnel enroll --verbose --jwt ${JWT_FILE} --identity "${IDENTITY_FILE}" || {
                echo "ERROR: failed to enroll with token from stdin, got $(wc -c < ${JWT_FILE})B" >&2
                exit 1
            }
        else
            echo "DEBUG: ${NF_REG_NAME}.jwt and env var \$NF_REG_TOKEN not found" >&2
            exit 1
        fi
    fi
else
    echo "ERROR: need NF_REG_NAME env var to save the identity file in mounted volume as ${IDENTITY_FILE}" >&2
    exit 1
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
ziti-tunnel "${TUNNEL_OPTS[@]}" "${@}" &
ZITI_TUNNEL_PID=$!
wait $ZITI_TUNNEL_PID
