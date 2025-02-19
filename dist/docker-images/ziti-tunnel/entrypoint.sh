#!/usr/bin/env bash

#
# Copyright 2025 NetFoundry Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

set -o errexit -o nounset -o pipefail
# set -o xtrace

function alldone() {
    # if successfully sent to background then send SIGTERM to trigger a cleanup
    # of resolver config, tun devices and associated routes
    if [[ "${ZITI_TUNNEL_PID:-}" =~ ^[0-9]+$ ]] && kill -0 "$ZITI_TUNNEL_PID" &>/dev/null; then
        kill -TERM "$ZITI_TUNNEL_PID"
        # let entrypoint script exit after ziti tunnel PID
        kill -0 "$ZITI_TUNNEL_PID" && wait "$ZITI_TUNNEL_PID"
    fi
}
trap alldone SIGTERM SIGINT EXIT

unset \
    IDENTITY_FILE \
    JSON_FILES \
    JWT_CANDIDATE \
    JWT_FILE \
    TUNNEL_OPTS \
    TUNNEL_RUN_MODE

# adapt deprecated NF_REG_* env vars to undefined ZITI_* env vars
if [[ -z "${ZITI_IDENTITY_BASENAME:-}" ]]; then
    if [[ -n "${NF_REG_NAME:-}" ]]; then
        echo "WARN: replacing deprecated NF_REG_NAME with ZITI_IDENTITY_BASENAME=${NF_REG_NAME}"
        ZITI_IDENTITY_BASENAME="${NF_REG_NAME}"
    elif [[ -n "${IOTEDGE_DEVICEID:-}" ]]; then
        echo "WARN: replacing deprecated IOTEDGE_DEVICEID with ZITI_IDENTITY_BASENAME=${IOTEDGE_DEVICEID}"
        ZITI_IDENTITY_BASENAME="${IOTEDGE_DEVICEID}"
    fi
fi
if [[ -z "${ZITI_ENROLL_TOKEN:-}" && -n "${NF_REG_TOKEN:-}" ]]; then
    echo "WARN: replacing deprecated NF_REG_TOKEN with ZITI_ENROLL_TOKEN=${NF_REG_TOKEN}"
    ZITI_ENROLL_TOKEN="${NF_REG_TOKEN}"
fi
if [[ -z "${ZITI_IDENTITY_WAIT:-}" && -n "${NF_REG_WAIT:-}" ]]; then
    echo "WARN: replacing deprecated var NF_REG_WAIT with ZITI_IDENTITY_WAIT=${NF_REG_WAIT}"
    ZITI_IDENTITY_WAIT="${NF_REG_WAIT}"
fi

# assign default identity dir if not set in parent env; this is a writeable path within the container image
: "${ZITI_IDENTITY_DIR:="/netfoundry"}"

# if enrolled identity JSON is provided then write it to a file in the identities dir
if [[ -n "${ZITI_IDENTITY_JSON:-}" ]]; then
    if [[ -z "${ZITI_IDENTITY_BASENAME:-}" ]]; then
        ZITI_IDENTITY_BASENAME="ziti_id"
    fi
    IDENTITY_FILE="${ZITI_IDENTITY_DIR}/${ZITI_IDENTITY_BASENAME}.json"
    if [[ -s "${IDENTITY_FILE}" ]]; then
        echo "WARN: clobbering non-empty Ziti identity file ${IDENTITY_FILE} with contents of env var ZITI_IDENTITY_JSON" >&2
    fi
    echo "${ZITI_IDENTITY_JSON}" > "${IDENTITY_FILE}"
# if an enrollment token is provided then write it to a file in the identities dir so it will be found in the next step
# and used to enroll
elif [[ -n "${ZITI_ENROLL_TOKEN:-}" ]]; then
    if [[ -z "${ZITI_IDENTITY_BASENAME:-}" ]]; then
        ZITI_IDENTITY_BASENAME="ziti_id"
    fi
    JWT_FILE="${ZITI_IDENTITY_DIR}/${ZITI_IDENTITY_BASENAME}.jwt"
    if [[ -s "${JWT_FILE}" ]]; then
        echo "WARN: clobbering non-empty Ziti enrollment token file ${JWT_FILE} with contents of env var ZITI_ENROLL_TOKEN" >&2
    fi
    echo "${ZITI_ENROLL_TOKEN}" > "${JWT_FILE}"
# otherwise, assume the identities dir is a mounted volume with identity files or tokens
else
    if ! [[ -d "${ZITI_IDENTITY_DIR}" ]]; then
        echo "ERROR: need directory ${ZITI_IDENTITY_DIR} to find tokens and identities" >&2
        exit 1
    fi
fi

typeset -a TUNNEL_OPTS
# if identity basename is specified then look for an identity file with that name, else load all identities in the
# identities dir mountpoint
if [[ -n "${ZITI_IDENTITY_BASENAME:-}" ]]; then
    IDENTITY_FILE="${ZITI_IDENTITY_DIR}/${ZITI_IDENTITY_BASENAME}.json"
    TUNNEL_OPTS=("--identity" "${IDENTITY_FILE}")

    # if wait is specified then wait for the identity file or token to appear
    : "${ZITI_IDENTITY_WAIT:=3}"
    if [[ "${ZITI_IDENTITY_WAIT}" =~ ^[0-9]+$ ]]; then
        echo "DEBUG: waiting ${ZITI_IDENTITY_WAIT}s for ${IDENTITY_FILE} (or token) to appear"
    elif (( "${ZITI_IDENTITY_WAIT}" < 0 )); then
        echo "DEBUG: waiting forever for ${IDENTITY_FILE} (or token) to appear"
    else
        echo "ERROR: need integer for ZITI_IDENTITY_WAIT" >&2
        exit 1
    fi

    while (( ZITI_IDENTITY_WAIT > 0 || ZITI_IDENTITY_WAIT < 0 )); do
        # if non-empty identity file
        if [[ -s "${IDENTITY_FILE}" ]]; then
            echo "INFO: found identity file ${IDENTITY_FILE}"
            break 1
        # look for enrollment token
        else
            echo "INFO: identity file ${IDENTITY_FILE} does not exist"
            for dir in "/var/run/secrets/netfoundry.io/enrollment-token" \
                      "/enrollment-token" \
                      "${ZITI_IDENTITY_DIR}"; do
                JWT_CANDIDATE="${dir}/${ZITI_IDENTITY_BASENAME}.jwt"
                echo "INFO: looking for ${JWT_CANDIDATE}"
                if [[ -s "${JWT_CANDIDATE}" ]]; then
                    JWT_FILE="${JWT_CANDIDATE}"
                    break 1
                fi
            done
            if [[ -n "${JWT_FILE:-}" ]]; then
                echo "INFO: enrolling ${JWT_FILE}"
                ziti edge enroll --jwt "${JWT_FILE}" --out "${IDENTITY_FILE}" || {
                    echo "ERROR: failed to enroll with token from ${JWT_FILE} ($(wc -c < "${JWT_FILE}")B)" >&2
                    exit 1
                }
                break 1
            fi
        fi
        # decrement the wait seconds until zero or forever if negative
        (( ZITI_IDENTITY_WAIT-- ))
        sleep 1
    done
else
    typeset -a JSON_FILES
    mapfile -t JSON_FILES < <(ls -1 "${ZITI_IDENTITY_DIR}"/*.json 2>/dev/null || true)
    if [[ ${#JSON_FILES[*]} -gt 0 ]]; then
        echo "INFO: ZITI_IDENTITY_BASENAME not set, loading ${#JSON_FILES[*]} identities from ${ZITI_IDENTITY_DIR}"
        TUNNEL_OPTS=("--identity-dir" "${ZITI_IDENTITY_DIR}")
    else
        echo "ERROR: ZITI_IDENTITY_BASENAME not set and zero identities found in ${ZITI_IDENTITY_DIR}" >&2
        exit 1
    fi
fi

echo "DEBUG: evaluating positionals: $*"
if (( ${#} )) && [[ ${1} =~ t?proxy|host ]]; then
    TUNNEL_RUN_MODE=${1}
    shift
else
    TUNNEL_RUN_MODE=tproxy
fi

echo "INFO: running \"ziti tunnel ${TUNNEL_RUN_MODE} ${TUNNEL_OPTS[*]} ${*}\""
ziti tunnel "${TUNNEL_RUN_MODE}" "${TUNNEL_OPTS[@]}" "${@}" &
ZITI_TUNNEL_PID=$!
wait $ZITI_TUNNEL_PID
