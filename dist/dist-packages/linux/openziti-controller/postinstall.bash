#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

install() {
    checkSystemdVersion "${MINIMUM_SYSTEMD_VERSION}"
    commonActions

}

upgrade() {
    # Step 2(upgrade), do what you need
    commonActions

}

commonActions() {
    makeEmptyRestrictedFile "${ZITI_PWD_FILE}"
    loadEnvStdin
    loadEnvFile
    promptBootstrap
    promptCtrlAdvertisedAddress
    promptCtrlPort
    promptPwd
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
        printf "\033[31m systemd version %s is less than %d , aborting \033[0m\n" "${systemd_version}" "$1"
        return 1
    fi
}

makeEmptyRestrictedFile() {
    if ! [ -s "$1" ]; then
        umask 0177
        touch "$1"
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
        echo "DEBUG: non-interactive, unable to prompt for answer: '$1'" >&3
        return 1
    fi
}

loadEnvStdin() {
    local key value
    # if not a tty (stdin is redirected), then slurp answers from stdin, e.g., env
    # assignments like ZITI_PWD=..., one per line
    if [[ ! -t 0 ]]; then
        while read -r line; do
            key=$(awk -F= '{print $1}' <<< "${line}")
            value=$(awk -F= '{print $2}' <<< "${line}")
            if [[ -n "${key}" && -n "${value}" ]]; then
                if grep -qE "^${key}=" "${ZITI_CTRL_BOOT_ENV_FILE}"; then
                    sed -Ei "s/^(${key})=.*/\1=${value}/" "${ZITI_CTRL_BOOT_ENV_FILE}"
                elif grep -qE "^${key}=" "${ZITI_CTRL_SVC_ENV_FILE}"; then
                    sed -Ei "s/^(${key})=.*/\1=${value}/" "${ZITI_CTRL_SVC_ENV_FILE}"
                else
                    echo "${key}=${value}" >> "${ZITI_CTRL_BOOT_ENV_FILE}"
                fi
            fi
        done
    fi
}

loadEnvFile() {
    # shellcheck disable=SC1090
    source "${ZITI_CTRL_SVC_ENV_FILE}"
    # shellcheck disable=SC1090
    source "${ZITI_CTRL_BOOT_ENV_FILE}"
}

promptCtrlAdvertisedAddress() {
    if [ -z "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" ]; then
        if ZITI_CTRL_ADVERTISED_ADDRESS="$(prompt "Enter the advertised address for the controller (FQDN) [$DEFAULT_ADDR]: " || echo "$DEFAULT_ADDR")"; then
            if [ -n "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" ]; then
                sed -Ei "s/^(ZITI_CTRL_ADVERTISED_ADDRESS)=.*/\1=${ZITI_CTRL_ADVERTISED_ADDRESS}/" "${ZITI_CTRL_BOOT_ENV_FILE}"
            fi
        else
            echo "WARN: missing ZITI_CTRL_ADVERTISED_ADDRESS in ${ZITI_CTRL_BOOT_ENV_FILE}" >&2
        fi
    fi
}

promptPwd() {
    # make ziti vars available in "ziti create config environment"
    exportZitiVars
    # shellcheck disable=SC1090 # compute the path to the identity file
    source <(ZITI_HOME=/var/lib/ziti-controller ziti create config environment)
    # do nothing if identity file has stuff in it
    if [ -s "${ZITI_CTRL_DATABASE_FILE}" ]; then
        echo "INFO: database exists in ${ZITI_CTRL_DATABASE_FILE}"
    # prompt for password token if interactive, unless already answered
    else
        if ! [[ "${ZITI_BOOTSTRAP_DATABASE:-}" == true ]]; then
            echo "INFO: ZITI_BOOTSTRAP_DATABASE is not true in ${ZITI_CTRL_SVC_FILE}" >&2
        # do nothing if enrollment token is already defined in env file
        elif [[ -n "${ZITI_PWD:-}" ]]; then
            echo "INFO: ZITI_PWD is defined in ${ZITI_CTRL_BOOT_ENV_FILE} and will be used to init db during"\
                    "next startup"
        elif    grep -qE "^LoadCredential=ZITI_PWD:${ZITI_PWD_FILE}" "${ZITI_CTRL_SVC_FILE}" \
                && [[ -s "${ZITI_PWD_FILE}" ]]; then
            echo "INFO: ZITI_PWD is defined in ${ZITI_PWD_FILE} and will be used to"\
                    "init db during next startup "
        else
            if ZITI_PWD="$(prompt "Enter the admin password: ")"; then
                if [ -n "${ZITI_PWD:-}" ]; then
                    echo "$ZITI_PWD" >| "${ZITI_PWD_FILE}"
                fi
            else
                echo "WARN: missing ZITI_PWD; use LoadCredential in"\
                        "${ZITI_CTRL_SVC_FILE} or set in ${ZITI_CTRL_BOOT_ENV_FILE}" >&2
            fi
        fi
    fi
}

promptBootstrap() {
    # if undefined, check previous answer in service unit or prompt for bootstrap, preserving default if no answer
    if [[ -z "${ZITI_BOOTSTRAP:-}" ]]; then
        if ZITI_BOOTSTRAP="$(prompt 'Generate a default config [y/N]: ' || echo 'false')"; then
            if [[ "${ZITI_BOOTSTRAP}" =~ ^[yY]([eE][sS])?$ ]]; then
                ZITI_BOOTSTRAP=true
            elif [[ "${ZITI_BOOTSTRAP}" =~ ^[nN][oO]?$ ]]; then
                ZITI_BOOTSTRAP=false
            fi
        fi
        sed -Ei "s/^(ZITI_BOOTSTRAP)=.*/\1=${ZITI_BOOTSTRAP}/" "${ZITI_CTRL_SVC_ENV_FILE}"
    fi
    if [[ "${ZITI_BOOTSTRAP}" != true ]]; then
        exit 0
    fi
}

promptCtrlPort() {
    # if undefined or default value in env file, prompt for router port, preserving default if no answer
    if [[ -z "${ZITI_CTRL_ADVERTISED_PORT:-}" ]]; then
        if ZITI_CTRL_ADVERTISED_PORT="$(prompt 'Enter the controller port [1280]: ' || echo '1280')"; then
            sed -Ei "s/^(ZITI_CTRL_ADVERTISED_PORT)=.*/\1=${ZITI_CTRL_ADVERTISED_PORT}/" "${ZITI_CTRL_BOOT_ENV_FILE}"
        fi
    fi
    if [[ "${ZITI_CTRL_ADVERTISED_PORT}" -le 1024 ]]; then
        grantNetBindService
    fi
}

grantNetBindService() {
    # grant binding privileged low ports unless already granted
    if ! grep -qE '^AmbientCapabilities=CAP_NET_BIND_SERVICE' "${ZITI_CTRL_SVC_FILE}"; then
        # uncomment the line
        sed -Ei 's/.*AmbientCapabilities=CAP_NET_BIND_SERVICE/AmbientCapabilities=CAP_NET_BIND_SERVICE/' "${ZITI_CTRL_SVC_FILE}"
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

: "${MINIMUM_SYSTEMD_VERSION:=232}"
DEFAULT_ADDR=localhost
ZITI_PWD_FILE=/opt/openziti/etc/controller/.pwd
ZITI_CTRL_SVC_ENV_FILE=/opt/openziti/etc/controller/service.env
ZITI_CTRL_BOOT_ENV_FILE=/opt/openziti/etc/controller/bootstrap.env
ZITI_CTRL_SVC_FILE=/lib/systemd/system/ziti-controller.service

# initialize a file descriptor for debug output
: "${DEBUG:=0}"
if (( DEBUG )); then
    exec 3>&1
    set -o xtrace
else
    exec 3>/dev/null
fi

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
