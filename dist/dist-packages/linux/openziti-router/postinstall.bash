#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail
# set -o xtrace

install() {
    checkSystemdVersion $MINIMUM_SYSTEMD_VERSION
    commonActions

}

upgrade() {
    # Step 2(upgrade), do what you need
    commonActions

}

commonActions() {
    makeTokenFile
    loadEnv
    promptCtrlAdvertisedAddress
    promptRouterAdvertisedAddress
    promptEnrollToken
    promptRouterMode
    promptRouterPort
}

checkSystemdVersion() {
    # Step 2 (clean install), enable the service in the proper way for this platform
    if ! command -V systemctl &>/dev/null; then
        echo "ERROR: required command 'systemctl' is missing" >&2
        return 1
    else
        systemd_version=$(systemctl --version | awk '/^systemd/ {print $2}')
    fi

    if [ "${systemd_version}" -lt "$1" ]; then
        printf "\033[31m systemd version %s is less then 232, aborting \033[0m\n" "${systemd_version}"
        return 1
    fi
}

makeTokenFile() {
    # unless it exists, create an empty enrollment token file with restrictive permissions so the service can start with
    # LoadCredential enabled
    if ! [ -s "${ZITI_ENROLL_TOKEN_FILE}" ]; then
        umask 0177
        touch "${ZITI_ENROLL_TOKEN_FILE}"
    fi
}

prompt() {
    # return true if interactive and response is not empty
    if [[ "${DEBIAN_FRONTEND:-}" != "noninteractive" && -t 0 ]]; then
        read -r -p "$1" response
        if [ -n "${response:-}" ]; then
            echo "${response}"
        else
            return 1
        fi
    else
        echo "WARN: non-interactive, unable to prompt for answer: '$1'" >&2
        return 1
    fi
}

loadEnv() {
    # shellcheck disable=SC1091
    source /opt/openziti/etc/router/env
}

promptCtrlAdvertisedAddress() {
    if [ -z "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" ]; then
        if ZITI_CTRL_ADVERTISED_ADDRESS="$(prompt 'Enter the advertised address for the controller: ')"; then
            if [ -n "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" ]; then
                sed -Ei "s/^(ZITI_CTRL_ADVERTISED_ADDRESS)=.*/\1=${ZITI_CTRL_ADVERTISED_ADDRESS}/" /opt/openziti/etc/router/env
            fi
        else
            echo "WARN: missing ZITI_CTRL_ADVERTISED_ADDRESS in /opt/openziti/etc/router/env" >&2
        fi
    fi
}

promptRouterAdvertisedAddress() {
    if [ -z "${ZITI_ROUTER_ADVERTISED_ADDRESS:-}" ]; then
        DEFAULT_ADDR="${HOSTNAME:=$(hostname -f)}"
        if ZITI_ROUTER_ADVERTISED_ADDRESS="$(prompt "Enter the advertised address for this router [$DEFAULT_ADDR]: " || echo "$DEFAULT_ADDR")"; then
            sed -Ei "s/^(ZITI_ROUTER_ADVERTISED_ADDRESS)=.*/\1=${ZITI_ROUTER_ADVERTISED_ADDRESS}/" /opt/openziti/etc/router/env
        fi
    fi
}

promptEnrollToken() {
    # make ziti vars available in "ziti create config environment"
    exportZitiVars
    # shellcheck disable=SC1090 # compute the path to the identity file
    source <(ZITI_HOME=/var/lib/ziti-router ziti create config environment)
    # do nothing if identity file has stuff in it
    if [ -s "${ZITI_ROUTER_IDENTITY_CERT}" ]; then
        echo "INFO: enrolled identity exists in ${ZITI_ROUTER_IDENTITY_CERT}"
    # prompt for enrollment token if interactive, unless already answered
    else
        ZITI_BOOTSTRAP_ENROLLMENT=$(awk -F= '/^Environment=ZITI_BOOTSTRAP_ENROLLMENT=/ {print $3}' /lib/systemd/system/ziti-router.service)
        if ! [[ "${ZITI_BOOTSTRAP_ENROLLMENT:-}" == true ]]; then
            echo "INFO: ZITI_BOOTSTRAP_ENROLLMENT is not true in /lib/systemd/system/ziti-router.service" >&2
        # do nothing if enrollment token is already defined in env file
        elif [[ -n "${ZITI_ENROLL_TOKEN:-}" ]]; then
            echo "INFO: ZITI_ENROLL_TOKEN is defined in /opt/openziti/etc/router/env and will be used to enroll during"\
                    "next startup"
        elif grep -qE "^LoadCredential=ZITI_ENROLL_TOKEN:${ZITI_ENROLL_TOKEN_FILE}" \
                    /lib/systemd/system/ziti-router.service \
                && [[ -s "${ZITI_ENROLL_TOKEN_FILE}" ]]; then
            echo "INFO: ZITI_ENROLL_TOKEN is defined in ${ZITI_ENROLL_TOKEN_FILE} and will be used to"\
                    "enroll during next startup "
        elif grep -qE '^SetCredential=ZITI_ENROLL_TOKEN:.+' /lib/systemd/system/ziti-router.service; then
            echo "INFO: ZITI_ENROLL_TOKEN is defined in /lib/systemd/system/ziti-router.service and will be used to"\
                    "enroll during next startup"
        else
            if ZITI_ENROLL_TOKEN=$(prompt "Enter the enrollment token: "); then
                if [ -n "${ZITI_ENROLL_TOKEN:-}" ]; then
                    echo "$ZITI_ENROLL_TOKEN" >| /opt/openziti/etc/router/.token
                fi
            else
                echo "WARN: missing ZITI_ENROLL_TOKEN; use LoadCredential or SetCredential in"\
                        "/lib/systemd/system/ziti-router.service or set in /opt/openziti/etc/router/env" >&2
            fi
        fi
    fi
}

promptRouterMode() {
    # if undefined or default value in env file, prompt for router mode, preserving default if no answer
    if [[ -z "${ZITI_ROUTER_MODE:-}" ]]; then
        if ZITI_ROUTER_MODE="$(prompt 'Enter the router mode (eg. host, tproxy, proxy) [host]: ' || echo 'host')"; then
            sed -Ei "s/^(ZITI_ROUTER_MODE)=.*/\1=${ZITI_ROUTER_MODE}/" /opt/openziti/etc/router/env
        fi
    fi
    # grant kernel capability NET_ADMIN if tproxy mode
    if [[ "${ZITI_ROUTER_MODE}" == tproxy ]]; then
        grantNetAdmin
        # also grant NET_BIND_SERVICE if resolver port is default 53 or defined <= 1024
        RESOLVER_PORT="${ZITI_ROUTER_TPROXY_RESOLVER##*:}"
        if [[ -z "${RESOLVER_PORT}" || "${RESOLVER_PORT}" -le 1024 ]]; then
            grantNetBindService
        fi
    fi
}

grantNetAdmin() {
    # grant ambient capabilities to the router process if not already granted
    if ! grep -qE '^AmbientCapabilities=CAP_NET_ADMIN' /lib/systemd/system/ziti-router.service; then
        # uncomment the line
        sed -Ei 's/.*AmbientCapabilities=CAP_NET_ADMIN/AmbientCapabilities=CAP_NET_ADMIN/' /lib/systemd/system/ziti-router.service
    fi
    systemctl daemon-reload
}

promptRouterPort() {
    # if undefined or default value in env file, prompt for router port, preserving default if no answer
    if [[ -z "${ZITI_ROUTER_PORT:-}" ]]; then
        if ZITI_ROUTER_PORT="$(prompt 'Enter the router port [3022]: ' || echo '3022')"; then
            sed -Ei "s/^(ZITI_ROUTER_PORT)=.*/\1=${ZITI_ROUTER_PORT}/" /opt/openziti/etc/router/env
        fi
    fi
    if [[ "${ZITI_ROUTER_PORT}" -le 1024 ]]; then
        grantNetBindService
    fi
}

grantNetBindService() {
    # grant binding privileged low ports unless already granted
    if ! grep -qE '^AmbientCapabilities=CAP_NET_BIND_SERVICE' /lib/systemd/system/ziti-router.service; then
        # uncomment the line
        sed -Ei 's/.*AmbientCapabilities=CAP_NET_BIND_SERVICE/AmbientCapabilities=CAP_NET_BIND_SERVICE/' /lib/systemd/system/ziti-router.service
    fi
    systemctl daemon-reload
}

exportZitiVars() {
    # make ziti vars available in forks like "ziti create config environment"
    for line in $(set | grep -e "^ZITI_" | sort); do
        # shellcheck disable=SC2013
        for var in $(awk -F= '{print $1}' <<< "$line"); do
            # shellcheck disable=SC2163
            export "$var"
        done
    done
}

MINIMUM_SYSTEMD_VERSION=232
ZITI_ENROLL_TOKEN_FILE=/opt/openziti/etc/router/.token

# Step 1, check if this is a clean install or an upgrade
if (( $# )); then
    if  [[ $1 == 1 || ($1 == configure && -z ${2:-}) ]]; then
        # deb passes $1=configure, rpm passes $1=1
        action=install
    elif [[ $1 == 2 || ($1 == configure && -n ${2:-}) ]]; then
        # deb passes $1=configure $2=<current version>, rpm passes $1=2
        action=upgrade
    else
        echo "ERROR: unexpected action '$1'" >&2
        exit 1
    fi
else
    echo "ERROR: missing action" >&2
    exit 1
fi

case "$action" in
    "install")
        printf "\033[32m Post Install of an clean install\033[0m\n"
        install
        ;;
    "upgrade")
        printf "\033[32m Post Install of an upgrade\033[0m\n"
        upgrade
        ;;
esac
