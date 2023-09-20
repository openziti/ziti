#!/usr/bin/env bash

#
# Copyright 2023 NetFoundry Inc.
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

set -e -u -o pipefail

function alldone() {
    # if successfully sent to background then send SIGTERM to trigger a cleanup
    # of resolver config, tun devices and associated routes
    [[ "${ZITI_TUNNEL_PID:-}" =~ ^[0-9]+$ ]] && {
        kill -TERM "$ZITI_TUNNEL_PID"
        # let entrypoint script exit after ziti tunnel PID
        kill -0 "$ZITI_TUNNEL_PID" && wait "$ZITI_TUNNEL_PID"
    }
}
trap alldone SIGTERM SIGINT EXIT

IDENTITIES_DIR="/netfoundry"
if ! [[ -d "${IDENTITIES_DIR}" ]]; then
    echo "ERROR: need directory ${IDENTITIES_DIR} to find tokens and identities" >&2
    exit 1
fi

if ! mountpoint "${IDENTITIES_DIR}" &>/dev/null; then
    echo "WARN: the identities directory is only available inside this container because ${IDENTITIES_DIR} is not a mounted volume. Be careful to not publish this image with identity inside or lose access to the identity by removing the image prematurely." >&2
else
    if [[ -n "${ZITI_IDENTITY_JSON:-}" ]]; then
        echo "WARNING: you supplied the Ziti identity as an env var and you mounted a volume on the identities dir. You may avoid this warning and future errors by not mounting a volume on ${IDENTITIES_DIR} when ZITI_IDENTITY_JSON is defined." >&2
    fi
fi

#
## Map the preferred, Ziti var names to legacy NF names. This allows us to begin using the preferred vars right away 
##  while minimizing immediate differences to the main control structure. This eases code review. Later, the legacy
##  names can be retired and replaced.
#
if [[ -n "${ZITI_IDENTITY_BASENAME:-}" ]]; then
    echo "INFO: setting NF_REG_NAME to \${ZITI_IDENTITY_BASENAME} (${ZITI_IDENTITY_BASENAME})"
    NF_REG_NAME="${ZITI_IDENTITY_BASENAME}"
fi
if [[ -n "${ZITI_ENROLL_TOKEN:-}" ]]; then
    echo "INFO: setting NF_REG_TOKEN to \${ZITI_ENROLL_TOKEN} (${ZITI_ENROLL_TOKEN})"
    NF_REG_TOKEN="${ZITI_ENROLL_TOKEN}"
fi
if [[ -n "${ZITI_IDENTITY_WAIT:-}" ]]; then
    echo "INFO: setting NF_REG_WAIT to \${ZITI_IDENTITY_WAIT} (${ZITI_IDENTITY_WAIT})"
    NF_REG_WAIT="${ZITI_IDENTITY_WAIT}"
fi

# treat IOTEDGE_DEVICEID, a standard var assigned by Azure IoT, as an alias for NF_REG_NAME
if [[ -z "${NF_REG_NAME:-}" ]]; then
    if [[ -n "${IOTEDGE_DEVICEID:-}" ]]; then
        echo "INFO: setting NF_REG_NAME to \${IOTEDGE_DEVICEID} (${IOTEDGE_DEVICEID})"
        NF_REG_NAME="${IOTEDGE_DEVICEID}"
    fi
fi

# if identity JSON var is defined then write to a file
if [[ -n "${ZITI_IDENTITY_JSON:-}" ]]; then
    # if the basename is not defined then use a default basename to write JSON to a file
    if [[ -z "${NF_REG_NAME:-}" ]]; then
        NF_REG_NAME="ziti_id"
    fi
    if [[ -s "${IDENTITIES_DIR}/${NF_REG_NAME}.json" ]]; then
        echo "ERROR: refusing to clobber non-empty Ziti identity file ${NF_REG_NAME}.json with contents of env var ZITI_IDENTITY_JSON!" >&2
        exit 1
    else
        echo "${ZITI_IDENTITY_JSON}" > "${IDENTITIES_DIR}/${NF_REG_NAME}.json"
    fi
fi

typeset -a TUNNEL_OPTS
# if identity file, else multiple identities dir
if [[ -n "${NF_REG_NAME:-}" ]]; then
    IDENTITY_FILE="${IDENTITIES_DIR}/${NF_REG_NAME}.json"
    TUNNEL_OPTS=("--identity" "${IDENTITY_FILE}")
    : ${NF_REG_WAIT:=1}
    if [[ "${NF_REG_WAIT}" =~ ^[0-9]+$ ]]; then
        echo "DEBUG: waiting ${NF_REG_WAIT}s for ${IDENTITY_FILE} (or token) to appear"
    elif (( "${NF_REG_WAIT}" < 0 )); then
        echo "DEBUG: waiting forever for ${IDENTITY_FILE} (or token) to appear"
    else
        echo "ERROR: need integer for NF_REG_WAIT" >&2
        exit 1
    fi
    while (( $NF_REG_WAIT > 0 || $NF_REG_WAIT < 0)); do
        # if non-empty identity file
        if [[ -s "${IDENTITY_FILE}" ]]; then
            echo "INFO: found identity file ${IDENTITY_FILE}"
            break 1
        # look for enrollment token
        else
            echo "INFO: identity file ${IDENTITY_FILE} does not exist"
            for dir in  "/var/run/secrets/netfoundry.io/enrollment-token" \
                        "/enrollment-token" \
                        "${IDENTITIES_DIR}"; do
                JWT_CANDIDATE="${dir}/${NF_REG_NAME}.jwt"
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
            elif [[ -n "${NF_REG_TOKEN:-}" ]]; then
                echo "INFO: attempting enrollment with NF_REG_TOKEN"
                if [[ -s "${IDENTITIES_DIR}/${NF_REG_NAME}.jwt" ]]; then
                    echo "ERROR: refusing to clobber non-empty enrollment OTP file ${NF_REG_NAME}.jwt with contents of env var NF_REG_TOKEN!" >&2
                    exit 1
                else
                    echo "${NF_REG_TOKEN}" > "${IDENTITIES_DIR}/${NF_REG_NAME}.jwt"
                fi
                ziti edge enroll --jwt "${IDENTITIES_DIR}/${NF_REG_NAME}.jwt" --out "${IDENTITY_FILE}" || {
                    echo "ERROR: failed to enroll with token from NF_REG_TOKEN ($(wc -c <<<"${NF_REG_TOKEN}")B)" >&2
                    exit 1
                }
            fi
        fi
        # decrement the wait seconds until zero or forever if negative
        (( NF_REG_WAIT-- ))
        sleep 1
    done
else
    typeset -a JSON_FILES
    mapfile -t JSON_FILES < <(ls -1 "${IDENTITIES_DIR}"/*.json)
    if [[ ${#JSON_FILES[*]} -gt 0 ]]; then
        echo "INFO: NF_REG_NAME not set, loading ${#JSON_FILES[*]} identities from ${IDENTITIES_DIR}"
        TUNNEL_OPTS=("--identity-dir" "${IDENTITIES_DIR}")
    else
        echo "ERROR: NF_REG_NAME not set and zero identities found in ${IDENTITIES_DIR}" >&2
        exit 1
    fi
fi

echo "DEBUG: evaluating positionals: $*"
if (( ${#} )) && [[ ${1} =~ t?proxy|host ]]; then
    TUNNEL_RUN_MODE=${1}
    shift
else
    TUNNEL_RUN_MODE=run
fi

echo "INFO: running \"ziti tunnel ${TUNNEL_RUN_MODE} ${TUNNEL_OPTS[*]} ${*}\""
ziti tunnel "${TUNNEL_RUN_MODE}" "${TUNNEL_OPTS[@]}" "${@}" &
ZITI_TUNNEL_PID=$!
wait $ZITI_TUNNEL_PID
