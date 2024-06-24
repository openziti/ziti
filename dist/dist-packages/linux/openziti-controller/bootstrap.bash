#!/usr/bin/env bash

#
# bootstrap the OpenZiti Controller with PKI, config file, and database
#

makePki() {
  #
  # create root and intermediate CA
  #

  # used by "ziti pki create server" as DNS SAN
  if [ -z "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" ]; then
    echo "ERROR: ZITI_CTRL_ADVERTISED_ADDRESS must be set, i.e., the FQDN by which all devices will reach the"\
    "controller and verify the server certificate" >&2
    return 1
  fi

  if [ "$ZITI_CA_FILE" == "$ZITI_INTERMEDIATE_FILE" ]; then
    echo "ERROR: ZITI_CA_FILE and ZITI_INTERMEDIATE_FILE must be different" >&2
    return 1
  fi

  ZITI_CA_CERT="${ZITI_PKI_ROOT}/${ZITI_CA_FILE}/certs/${ZITI_CA_FILE}.cert"
  if [ ! -s "${ZITI_CA_CERT}" ]; then
    ziti pki create ca \
      --pki-root "${ZITI_PKI_ROOT}" \
      --ca-file "${ZITI_CA_FILE}"
  fi

  ZITI_PKI_SIGNER_CERT="${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/certs/${ZITI_INTERMEDIATE_FILE}.cert"
  ZITI_PKI_SIGNER_KEY="${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/keys/${ZITI_INTERMEDIATE_FILE}.key"
  if [[ ! -s "$ZITI_PKI_SIGNER_CERT" && ! -s "$ZITI_PKI_SIGNER_KEY" ]]; then
    ziti pki create intermediate \
      --pki-root "${ZITI_PKI_ROOT}" \
      --ca-name "${ZITI_CA_FILE}" \
      --intermediate-file "${ZITI_INTERMEDIATE_FILE}"
  elif [[ ! -s "$ZITI_PKI_SIGNER_CERT" || ! -s "$ZITI_PKI_SIGNER_KEY" ]]; then
    echo "ERROR: $ZITI_PKI_SIGNER_CERT and $ZITI_PKI_SIGNER_KEY must both exist or neither exist as non-empty files" >&2
    return 1
  fi

  #
  # create server and client keys
  #

  if [ "$ZITI_SERVER_FILE" == "$ZITI_CLIENT_FILE" ]; then
    echo "ERROR: ZITI_SERVER_FILE and ZITI_CLIENT_FILE must be different" >&2
    return 1
  fi

  ZITI_PKI_CTRL_KEY="${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/keys/${ZITI_SERVER_FILE}.key"
  if ! [ -s "$ZITI_PKI_CTRL_KEY" ]; then
    ziti pki create key \
      --pki-root "${ZITI_PKI_ROOT}" \
      --ca-name "${ZITI_INTERMEDIATE_FILE}" \
      --key-file "${ZITI_SERVER_FILE}"
  fi

  # use the server key for both client and server certs until "ziti create config controller" supports separate keys for
  # each
  # CLIENT_KEY_FILE="${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/keys/${ZITI_CLIENT_FILE}.key"
  # if ! [ -s "$CLIENT_KEY_FILE" ]; then
  #   ziti pki create key \
  #     --pki-root "${ZITI_PKI_ROOT}" \
  #     --ca-name "${ZITI_INTERMEDIATE_FILE}" \
  #     --key-file "${ZITI_CLIENT_FILE}"
  # fi

  #
  # create server and client certs
  #

  # server cert
  ZITI_PKI_CTRL_SERVER_CERT="${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/certs/${ZITI_SERVER_FILE}.chain.pem"
  if [[ "${ZITI_AUTO_RENEW_CERTS}" == true || ! -s "$ZITI_PKI_CTRL_SERVER_CERT" ]]; then
    # server cert
    ziti pki create server \
      --pki-root "${ZITI_PKI_ROOT}" \
      --ca-name "${ZITI_INTERMEDIATE_FILE}" \
      --key-file "${ZITI_SERVER_FILE}" \
      --server-file "${ZITI_SERVER_FILE}" \
      --dns "${ZITI_CTRL_ADVERTISED_ADDRESS}" \
      --allow-overwrite
  fi

  # client cert
  #   use the server key for both client and server certs until "ziti create config controller" supports separate keys for
  #   each
  ZITI_PKI_CTRL_CERT="${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/certs/${ZITI_CLIENT_FILE}.cert"
  if [[ "${ZITI_AUTO_RENEW_CERTS}" == true || ! -s "$ZITI_PKI_CTRL_CERT" ]]; then
    ziti pki create client \
      --pki-root "${ZITI_PKI_ROOT}" \
      --ca-name "${ZITI_INTERMEDIATE_FILE}" \
      --key-file "${ZITI_SERVER_FILE}" \
      --client-file "${ZITI_CLIENT_FILE}" \
      --allow-overwrite
  fi

}

makeDatabase() {

  #
  # create default admin in database
  #

  if [ -s "${ZITI_CTRL_DATABASE_FILE}" ]; then
    return 0
  fi

  # if the database file is in a subdirectory, create the directory so that "ziti controller edge init" can load the
  # controller config.yml which contains a check to ensure the directory exists
  DB_DIR="$(dirname "${ZITI_CTRL_DATABASE_FILE}")"
  if ! [ "$DB_DIR" == "." ]; then
    mkdir -p "$DB_DIR"
  fi

  : "${ZITI_PWD:=$(< "${ZITI_PWD_FILE}")}"
  if [ -n "${ZITI_PWD}" ]; then
    if ! ziti controller edge init "${ZITI_CTRL_CONFIG_FILE}" \
      --username "${ZITI_USER}" \
      --password "${ZITI_PWD}"
    then
      echo "ERROR: failed to create default admin in database" >&2
      # do not leave behind a partially-initialized database file because it prevents us from trying again
      rm -f "${ZITI_CTRL_DATABASE_FILE}"
      return 1
    fi
  else
    echo  "ERROR: need admin password in ${ZITI_PWD_FILE} or env var ZITI_PWD" >&2
    return 1
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

loadEnvFiles() {
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
  # do nothing if database file has stuff in it
  if [ -s "${ZITI_CTRL_DATABASE_FILE}" ]; then
      echo "INFO: database exists in ${ZITI_CTRL_DATABASE_FILE}"
  # prompt for password token if interactive, unless already answered
  else
  if ! [[ "${ZITI_BOOTSTRAP_DATABASE:-}" == true ]]; then
    echo "INFO: ZITI_BOOTSTRAP_DATABASE is not true in ${ZITI_CTRL_SVC_ENV_FILE}" >&2
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

setBootstrapEnabled() {
  if [[ -z "${ZITI_BOOTSTRAP:-}" ]]; then
    sed -Ei "s/^(ZITI_BOOTSTRAP)=.*/\1=true/" "${ZITI_CTRL_SVC_ENV_FILE}"
  fi
}

promptCtrlPort() {
  # if undefined or default value in env file, prompt for router port, preserving default if no answer
  if [[ -z "${ZITI_CTRL_ADVERTISED_PORT:-}" ]]; then
    if ZITI_CTRL_ADVERTISED_PORT="$(prompt 'Enter the controller port [1280]: ' || echo '1280')"; then
      sed -Ei "s/^(ZITI_CTRL_ADVERTISED_PORT)=.*/\1=${ZITI_CTRL_ADVERTISED_PORT}/" "${ZITI_CTRL_BOOT_ENV_FILE}"
    fi
  fi
  if [[ "${ZITI_CTRL_ADVERTISED_PORT}" -lt 1024 ]]; then
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
  # make ziti vars available in forks like "ziti create config controller"
  for line in $(set | grep -e "^ZITI_" | sort); do
    # shellcheck disable=SC2013
    for var in $(awk -F= '{print $1}' <<< "$line"); do
      # shellcheck disable=SC2163
      export "$var"
    done
  done
}

makeConfig() {
  #
  # create config file
  #

  # enforce first argument is a non-empty string that does not begin with "--" (long option prefix)
  if [[ -n "${1:-}" && ! "${1}" =~ ^-- ]]; then
    local ZITI_CTRL_CONFIG_FILE="${1}"
    shift
  else
    echo "ERROR: no config file path provided" >&2
    return 1
  fi
  shopt -u nocasematch  # toggle on case-sensitive comparison

  # used by "ziti create config controller" as advertised address
  if [[ -z "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" ]] || ! printenv | grep -q ZITI_CTRL_ADVERTISED_ADDRESS &>/dev/null ; then
    echo "ERROR: ZITI_CTRL_ADVERTISED_ADDRESS must be set and exported, i.e., the FQDN by which all devices will reach the"\
    " controller and verify the server certificate" >&2
    return 1
  else
    echo "DEBUG: ZITI_CTRL_ADVERTISED_ADDRESS is set to ${ZITI_CTRL_ADVERTISED_ADDRESS}" >&3
  fi

  # set the path to the root CA cert
  export  ZITI_PKI_CTRL_CA="${ZITI_PKI_ROOT}/${ZITI_CA_FILE}/certs/${ZITI_CA_FILE}.cert"

  # set the URI of the edge-client API (uses same TCP port); e.g., ztAPI: ziti.example.com:1280
  export  ZITI_CTRL_EDGE_ADVERTISED_ADDRESS="${ZITI_CTRL_ADVERTISED_ADDRESS}" \
          ZITI_CTRL_EDGE_ADVERTISED_PORT="${ZITI_CTRL_ADVERTISED_PORT:=1280}"

  # export the vars that were assigned inside this script to set the path to the server and client certs and their common
  # private key, and the intermediate (signer) CA cert and key
  export  ZITI_PKI_EDGE_SERVER_CERT="${ZITI_PKI_CTRL_SERVER_CERT}" \
          ZITI_PKI_EDGE_CERT="${ZITI_PKI_CTRL_CERT}" \
          ZITI_PKI_EDGE_KEY="${ZITI_PKI_CTRL_KEY}" \
          ZITI_PKI_EDGE_CA="${ZITI_PKI_CTRL_CA}"

  if [[ ! -s "${ZITI_CTRL_CONFIG_FILE}" || "${1:-}" == --force ]]; then
    ziti create config controller \
      --output "${ZITI_CTRL_CONFIG_FILE}"
  fi

}

bootstrap() {

  if [ -n "${1:-}" ]; then
    local ZITI_CTRL_CONFIG_FILE="${1}"
    local ZITI_CTRL_CONFIG_DIR="$(dirname "${ZITI_CTRL_CONFIG_FILE}")"
    echo "DEBUG: using config file path: $(realpath "${ZITI_CTRL_CONFIG_FILE}")" >&3
  else
    echo "ERROR: no config file path provided" >&2
    return 1
  fi

  mkdir -pm0700 "${ZITI_CTRL_CONFIG_DIR}"
  cd "${ZITI_CTRL_CONFIG_DIR}"

  # make PKI unless explicitly disabled or it already exists
  if [ "${ZITI_BOOTSTRAP_PKI}"      == true ]; then
    makePki
  fi

  # make config file unless explicitly disabled or it exists, set "force" to overwrite
  if [ "${ZITI_BOOTSTRAP_CONFIG}"   == true ]; then
    makeConfig "${ZITI_CTRL_CONFIG_FILE}"
  elif [ "${ZITI_BOOTSTRAP_CONFIG}" == force ]; then
    makeConfig "${ZITI_CTRL_CONFIG_FILE}" --force
  fi

  # make database unless explicityly disabled or it exists
  if [ "${ZITI_BOOTSTRAP_DATABASE}" == true ]; then
    makeDatabase
  fi

  # disown root to allow systemd to manage the working directory as dynamic user
  chown -R "${ZIGGY_UID:-nobody}:${ZIGGY_GID:-nogroup}" "${ZITI_CTRL_CONFIG_DIR}/"
  chmod -R u=rwX,go-rwx "${ZITI_CTRL_CONFIG_DIR}/"
}

# BEGIN

# run the bootstrap function if this script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then

  set -o errexit
  set -o nounset
  set -o pipefail

  # initialize a file descriptor for debug output
  : "${DEBUG:=0}"
  if (( DEBUG )); then
    exec 3>&1
    set -o xtrace
  else
    exec 3>/dev/null
  fi

  DEFAULT_ADDR=localhost
  ZITI_CTRL_SVC_ENV_FILE=/opt/openziti/etc/controller/service.env
  ZITI_CTRL_BOOT_ENV_FILE=/opt/openziti/etc/controller/bootstrap.env
  ZITI_CTRL_SVC_FILE=/etc/systemd/system/ziti-controller.service.d/override.conf
  ZITI_PWD_FILE=/opt/openziti/etc/controller/.pwd
  : "${ZITI_HOME:=/var/lib/ziti-controller}"; export ZITI_HOME

  loadEnvStdin                  # if stdin is a terminal, load env from it
  loadEnvFiles                  # override stdin with ZITI_CTRL_SVC_ENV_FILE then ZITI_CTRL_BOOT_ENV_FILE
  promptCtrlAdvertisedAddress   # prompt for ZITI_CTRL_ADVERTISED_ADDRESS if not already set
  promptCtrlPort                # prompt for ZITI_CTRL_ADVERTISED_PORT if not already set
  promptPwd                     # prompt for ZITI_PWD if not already set
  loadEnvFiles                  # reload env files to source new answers from prompts
  exportZitiVars                # export all ZITI_ vars to be used in bootstrap

  if ! (( $# ))
  then
    set -- "${ZITI_HOME}/config.yml"
  fi
  bootstrap "${@}"

  # unless bootstrapping is explicitly disabled, ensure the toggle reflects the configuration is managed by this script
  # because bootstrapping was invoked directly and completed without error
  setBootstrapEnabled
fi
