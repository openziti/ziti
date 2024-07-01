#!/usr/bin/env bash

#
# bootstrap the OpenZiti Controller with PKI, config file, and database
#

makePki() {
  #
  # create root and intermediate CA
  #

  # used by "ziti pki create server" as DNS SAN
  if [[ -z "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" ]]; then
    echo "ERROR: ZITI_CTRL_ADVERTISED_ADDRESS must be set, i.e., the FQDN by which all devices will reach the"\
    "controller and verify the server certificate" >&2
    hintBootstrap "${PWD}"
    return 1
  fi

  if [[ "${ZITI_CA_FILE}" == "${ZITI_INTERMEDIATE_FILE}" ]]; then
    echo "ERROR: ZITI_CA_FILE and ZITI_INTERMEDIATE_FILE must be different" >&2
    return 1
  fi

  ZITI_CA_CERT="${ZITI_PKI_ROOT}/${ZITI_CA_FILE}/certs/${ZITI_CA_FILE}.cert"
  if [[ ! -s "${ZITI_CA_CERT}" ]]; then
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
  elif [[ ! -s "${ZITI_PKI_SIGNER_CERT}" || ! -s "${ZITI_PKI_SIGNER_KEY}" ]]; then
    echo "ERROR: ${ZITI_PKI_SIGNER_CERT} and ${ZITI_PKI_SIGNER_KEY} must both exist or neither exist as non-empty files" >&2
    return 1
  else
    echo "INFO: edge signer CA exists in $(realpath "${ZITI_PKI_SIGNER_CERT}")"
  fi

  #
  # create server and client keys
  #

  if [[ "${ZITI_SERVER_FILE}" == "${ZITI_CLIENT_FILE}" ]]; then
    echo "ERROR: ZITI_SERVER_FILE and ZITI_CLIENT_FILE must be different" >&2
    return 1
  fi

  ZITI_PKI_CTRL_KEY="${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/keys/${ZITI_SERVER_FILE}.key"
  if ! [[ -s "$ZITI_PKI_CTRL_KEY" ]]; then
    ziti pki create key \
      --pki-root "${ZITI_PKI_ROOT}" \
      --ca-name "${ZITI_INTERMEDIATE_FILE}" \
      --key-file "${ZITI_SERVER_FILE}"
  fi

  # use the server key for both client and server certs until "ziti create config controller" supports separate keys for
  # each
  # CLIENT_KEY_FILE="${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/keys/${ZITI_CLIENT_FILE}.key"
  # if ! [[ -s "$CLIENT_KEY_FILE" ]]; then
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
      --dns "localhost,${ZITI_CTRL_ADVERTISED_ADDRESS}" \
      --ip "127.0.0.1,::1" \
      --allow-overwrite >&3  # write to debug fd because this runs every startup
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
      --allow-overwrite >&3  # write to debug fd because this runs every startup
  fi

}

makeConfig() {
  #
  # create config file
  #

  # enforce first argument is a non-empty string that does not begin with "--" (long option prefix)
  if [[ -n "${1:-}" && ! "${1}" =~ ^-- ]]; then
    local _config_file="${1}"
    shift
  else
    echo "ERROR: no config file path provided" >&2
    return 1
  fi
  shopt -u nocasematch  # toggle on case-sensitive comparison

  # used by "ziti create config controller" as advertised address
  if [[ -z "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" ]]; then
    echo "ERROR: ZITI_CTRL_ADVERTISED_ADDRESS must be set; i.e., the FQDN by which all devices will reach the"\
    "controller and verify the server certificate" >&2
    hintBootstrap "${PWD}"
    return 1
  else
    echo "DEBUG: ZITI_CTRL_ADVERTISED_ADDRESS is set to ${ZITI_CTRL_ADVERTISED_ADDRESS}" >&3
  fi

  # set the path to the root CA cert
  export  ZITI_PKI_CTRL_CA="${ZITI_PKI_ROOT}/${ZITI_CA_FILE}/certs/${ZITI_CA_FILE}.cert"

  # set the URI of the edge-client API (uses same TCP port); e.g., ztAPI: ctrl.ziti.example.com:1280
  export  ZITI_CTRL_EDGE_ADVERTISED_ADDRESS="${ZITI_CTRL_ADVERTISED_ADDRESS}" \
          ZITI_CTRL_EDGE_ADVERTISED_PORT="${ZITI_CTRL_ADVERTISED_PORT:=1280}"

  # export the vars that were assigned inside this script to set the path to the server and client certs and their common
  # private key, and the intermediate (signer) CA cert and key
  export  ZITI_PKI_EDGE_SERVER_CERT="${ZITI_PKI_CTRL_SERVER_CERT}" \
          ZITI_PKI_EDGE_CERT="${ZITI_PKI_CTRL_CERT}" \
          ZITI_PKI_EDGE_KEY="${ZITI_PKI_CTRL_KEY}" \
          ZITI_PKI_EDGE_CA="${ZITI_PKI_CTRL_CA}"

  exportZitiVars                # export all ZITI_ vars to be used in bootstrap
  if [[ ! -s "${_config_file}" || "${1:-}" == --force ]]; then
    ziti create config controller \
      --output "${_config_file}"
  fi

}

makeDatabase() {

  #
  # create default admin in database
  #

  if [[ -n "${1:-}" ]]; then
    local _config_file="${1}"
  else
    echo "ERROR: no config file path provided" >&2
    return 1
  fi

  local _db_file
  _db_file=$(dbFile "${_config_file}")
  if [[ -s "${_db_file}" ]]; then
    echo "DEBUG: database file already exists: ${_db_file}" >&3
    return 0
  else
    echo "DEBUG: creating database file: ${_db_file}" >&3
  fi

  # if the database file is in a subdirectory, create the directory so that "ziti controller edge init" can load the
  # controller config.yml which contains a check to ensure the directory exists
  DB_DIR="$(dirname "${ZITI_CTRL_DATABASE_FILE}")"
  if ! [[ "$DB_DIR" == "." ]]; then
    mkdir -p "$DB_DIR"
  fi

  if [[ -n "${ZITI_USER:-}" && -n "${ZITI_PWD:-}" ]]; then
    if ziti controller edge init "${_config_file}" \
      --username "${ZITI_USER}" \
      --password "${ZITI_PWD}" 2>&1  # set VERBOSE=1 or DEBUG=1 to see the output
    then
      echo "DEBUG: created default admin in database" >&3
    else
      echo "ERROR: failed to create default admin in database" >&2
      # do not leave behind a partially-initialized database file because it prevents us from trying again
      rm -f "${ZITI_CTRL_DATABASE_FILE}"
      return 1
    fi
  else
    echo  "ERROR: unable to create default admin in database because ZITI_USER and ZITI_PWD must both be set" >&2
    hintBootstrap "${PWD}"
    return 1
  fi

}

isInteractive() {
  # return true if interactive and response is not empty
  if [[ "${DEBIAN_FRONTEND:-}" != "noninteractive" && -t 0 ]]; then
    return 0
  else
    return 1
  fi
}

prompt() {
  # return true if interactive and response is not empty
  if isInteractive; then
    read -r -p "$1" response
    if [[ -n "${response:-}" ]]; then
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
  # if not a tty (stdin is redirected), then slurp answers from stdin, e.g., env
  # assignments like ZITI_PWD=abcd1234, one per line
  if [[ ! -t 0 ]]; then
    while read -r line; do
      if [[ "${line:-}" =~ ^ZITI_.*= ]]; then
        eval "${line}"
        setAnswer "${line}" "${SVC_ENV_FILE}" "${BOOT_ENV_FILE}"
      # ignore lines beginning with # and lines containing only zero or more whitespace chars
      elif [[ "${line:-}" =~ ^(#|\\s*?$) ]]; then
        echo "DEBUG: ignoring '${line}'" >&3
        continue
      else
        echo "WARN: ignoring '${line}'; not a ZITI_* env var assignment" >&2
      fi
    done
  fi
}

loadEnvFiles() {
  for ENV_FILE in "${BOOT_ENV_FILE}" "${SVC_ENV_FILE}"
  do
    if [[ -s "${ENV_FILE}" ]]
    then
      # shellcheck disable=SC1090
      source "${ENV_FILE}"
    else
      echo "WARN: missing env file '${ENV_FILE}'"
    fi 
  done
}

promptCtrlAddress() {
  if [[ -z "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" ]]; then
    if ZITI_CTRL_ADVERTISED_ADDRESS="$(prompt "Enter DNS name of the controller [$DEFAULT_ADDR]: " || echo "$DEFAULT_ADDR")"; then
      if [[ -n "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" ]]; then
        setAnswer "ZITI_CTRL_ADVERTISED_ADDRESS=${ZITI_CTRL_ADVERTISED_ADDRESS}" "${BOOT_ENV_FILE}"
      fi
    else
      echo "WARN: missing ZITI_CTRL_ADVERTISED_ADDRESS in ${BOOT_ENV_FILE}" >&2
    fi
  fi
}


# if bootstrapping was previously explicitly disabled and running interactively then prompt for re-enable, preserving
# the currently disabled setting if non-interactive or no answer
promptBootstrap() {
    # do not prompt if unset or set to true because executing interactively means we want bootstrapping
    if [[ -n "${ZITI_BOOTSTRAP:-}" && "${ZITI_BOOTSTRAP}" != true ]]; then
        if ZITI_BOOTSTRAP="$(prompt 'Generate a default config [y/N]: ' || echo 'false')"; then
            if [[ "${ZITI_BOOTSTRAP}" =~ ^([yY]([eE][sS])?|[tT]([rR][uU][eE])?)$ ]]; then
                ZITI_BOOTSTRAP=true
            elif [[ "${ZITI_BOOTSTRAP}" =~ ^([nN][oO]?|[fF]([aA][lL][sS][eE])?)$ ]]; then
                ZITI_BOOTSTRAP=false
            fi
        fi
        setAnswer "ZITI_BOOTSTRAP=${ZITI_BOOTSTRAP}" "${SVC_ENV_FILE}"
    fi
    if [[ -n "${ZITI_BOOTSTRAP:-}" && "${ZITI_BOOTSTRAP}" != true ]]; then
        return 1
    fi
}

promptPwd() {
  # do nothing if database file has stuff in it
  if [[ -s "${ZITI_CTRL_DATABASE_FILE}" ]]; then
      echo "DEBUG: database exists in $(realpath "${ZITI_CTRL_DATABASE_FILE}")" >&3
  # prompt for password token if interactive, unless already answered
  else
    if ! [[ "${ZITI_BOOTSTRAP_DATABASE:-}" == true ]]; then
      echo "INFO: ZITI_BOOTSTRAP_DATABASE is not true in ${SVC_ENV_FILE}" >&2
    # do nothing if enrollment token is already defined in env file
    elif [[ -n "${ZITI_PWD:-}" ]]; then
      echo "DEBUG: ZITI_PWD is defined in ${BOOT_ENV_FILE} and will be used to init db during"\
            "next startup" >&3
    else
      GEN_PWD=$(head -c1024 /dev/urandom | LC_ALL=C tr -dc 'A-Za-z0-9!@#$%^*_+~' | cut -c 1-12)
      if isInteractive && ZITI_PWD="$(prompt "Set password for '${ZITI_USER}' [${GEN_PWD}]: " || echo "${GEN_PWD}")"; then
        if [[ -n "${ZITI_PWD:-}" ]]; then
          # temporarily set password in env file, then scrub after db init
          setAnswer "ZITI_PWD=${ZITI_PWD}" "${BOOT_ENV_FILE}"
        fi
      else
        echo "ERROR: ZITI_PWD is required" >&2
        return 1
      fi
    fi
fi
}

promptBindAddress() {
  if [[ -z "${ZITI_CTRL_BIND_ADDRESS:-}" ]]; then
    if ZITI_CTRL_BIND_ADDRESS="$(prompt "Enter the bind address for the controller [0.0.0.0]: " || echo '0.0.0.0')"; then
      if [[ -n "${ZITI_CTRL_BIND_ADDRESS:-}" ]]; then
        setAnswer "ZITI_CTRL_BIND_ADDRESS=${ZITI_CTRL_BIND_ADDRESS}" "${BOOT_ENV_FILE}"
      fi
    else
      echo "WARN: missing ZITI_CTRL_BIND_ADDRESS in ${BOOT_ENV_FILE}" >&2
    fi
  fi
}

promptUser() {
  if [[ -z "${ZITI_USER:-}" ]]; then
    if ZITI_USER="$(prompt "Enter the name of the default user [admin]: " || echo 'admin')"; then
      if [[ -n "${ZITI_USER:-}" ]]; then
        setAnswer "ZITI_USER=${ZITI_USER}" "${BOOT_ENV_FILE}"
      fi
    else
      echo "WARN: missing ZITI_USER in ${BOOT_ENV_FILE}" >&2
    fi
  fi
}

setBootstrapEnabled() {
  if [[ -z "${ZITI_BOOTSTRAP:-}" ]]; then
    setAnswer "ZITI_BOOTSTRAP=true" "${SVC_ENV_FILE}"
  fi
}

setAnswer() {
  if [[ "${#}" -ge 2 ]]; then
    local _key=${1%=*}
    local _value=${1#*=}
    # strip quotes
    _value="${_value//\"}"
    _value="${_value//\'}"
    shift
    local -a _env_files=("${@}")  # ordered list of files to seek a matching key to assign value
    for _env_file in "${_env_files[@]}"; do
      # do nothing if already set
      if grep -qE "^${_key}=['\"]?${_value}['\"]?[\s$]" "${_env_file}"; then
        return 0
      # set if unset
      elif grep -qE "^${_key}=" "${_env_file}"; then
        sed -Ei "s|^${_key}=.*|${_key}='${_value}'|g" "${_env_file}"
        return 0
      fi
    done
    # append to last file if none matched the key
    echo -e "\n${_key}='${_value}'" >> "${_env_files[${#_env_files[@]}-1]}"
  else
    echo "ERROR: setAnswer() requires at least two arguments, e.g., setAnswer 'ZITI_PWD=abcd1234' ./some1.env ./some2.env" >&2
    return 1
  fi
}

promptCtrlPort() {
  # if undefined or default value in env file, prompt for router port, preserving default if no answer
  if [[ -z "${ZITI_CTRL_ADVERTISED_PORT:-}" ]]; then
    if ZITI_CTRL_ADVERTISED_PORT="$(prompt 'Enter the controller port [1280]: ' || echo '1280')"; then
      setAnswer "ZITI_CTRL_ADVERTISED_PORT=${ZITI_CTRL_ADVERTISED_PORT}" "${BOOT_ENV_FILE}"
    fi
  fi
  if [[ "${ZITI_CTRL_ADVERTISED_PORT}" -lt 1024 ]]; then
    grantNetBindService
  fi
}

grantNetBindService() {
  # grant binding privileged low ports unless already granted
  if ! grep -qE '^AmbientCapabilities=CAP_NET_BIND_SERVICE' "${SVC_FILE}"; then
    # uncomment the line
    sed -Ei 's/.*(AmbientCapabilities=CAP_NET_BIND_SERVICE)/\1/' "${SVC_FILE}"
  fi
  systemctl daemon-reload
}

importZitiVars() {
  # inherit Ziti vars and set answers
  for line in $(set | grep -e "^ZITI_" | sort); do
    # shellcheck disable=SC2013
    setAnswer "${line}" "${SVC_ENV_FILE}" "${BOOT_ENV_FILE}"
  done
}

exportZitiVars() {
  # make ziti vars available in forks like "ziti create config controller"
  for line in $(set | grep -e "^ZITI_" | sort); do
    # shellcheck disable=SC2013
    export "${line%=*}"
  done
}

bootstrap() {

  if [[ "${ZITI_BOOTSTRAP:-}" != true ]]; then
    echo "WARN: not checking or generating a config because ZITI_BOOTSTRAP is not set to 'true'" >&2
    return 0
  fi

  if [[ -n "${1:-}" ]]; then
    local _ctrl_config_file="${1}"
    echo "DEBUG: using config: $(realpath "${_ctrl_config_file}")" >&3
  else
    echo "ERROR: no config file path provided" >&2
    return 1
  fi

  # make PKI unless explicitly disabled or it already exists
  if [[ "${ZITI_BOOTSTRAP_PKI}"      == true ]]; then
    if ! [[ -s "${_ctrl_config_file}" ]]; then
      # make PKI unless explicitly disabled or a configuration already exists
      makePki
    fi
  fi

  # make config file unless explicitly disabled or it exists, set "force" to overwrite
  if [[ -s "${_ctrl_config_file}" && "${ZITI_BOOTSTRAP_CONFIG}"   != force ]]; then
    echo "INFO: config file exists in $(realpath "${_ctrl_config_file}")"
  elif [[ "${ZITI_BOOTSTRAP_CONFIG}" == true ]]; then
    makeConfig "${_ctrl_config_file}"
  elif [[ "${ZITI_BOOTSTRAP_CONFIG}" == force ]]; then
    makeConfig "${_ctrl_config_file}" --force
  else
    echo "ERROR: unexpected value in ZITI_BOOTSTRAP_CONFIG=${ZITI_BOOTSTRAP_CONFIG}" >&2
    return 1
  fi

  # make database unless explicitly disabled or it exists
  if [[ "${ZITI_BOOTSTRAP_DATABASE}" == true ]]; then
    makeDatabase "${_ctrl_config_file}"
  fi

}

prepareWorkingDir() {
  if [[ -n "${1:-}" ]]; then
    local _config_dir="$1"
    echo "DEBUG: preparing working directory: $(realpath "${_config_dir}")" >&3
  else
    echo "ERROR: no working dir path provided" >&2
    return 1
  fi

  # shellcheck disable=SC2174
  mkdir -pm0700 "${_config_dir}"
  # set pwd for subesquent bootstrap command
  cd "${_config_dir}"
}

finalizeWorkingDir() {
  if [[ -n "${1:-}" ]]; then
    local _config_dir="$1"
    echo "DEBUG: finalizing working directory: $(realpath "${_config_dir}")" >&3
  else
    echo "ERROR: no working dir path provided" >&2
    return 1
  fi

  # disown root to allow systemd to manage the working directory as dynamic user
  chown -R "${ZIGGY_UID:-65534}:${ZIGGY_GID:-65534}" "${_config_dir}/"
  chmod -R u=rwX,go-rwx "${_config_dir}/"
}

hintBootstrap() {

  local _work_dir="${1:-${PWD}}"

  echo -e "\nProvide a configuration in '${_work_dir}' or generate with:"\
          "\n* Set vars in'/opt/openziti/etc/controller/bootstrap.env'"\
          "\n* Run '/opt/openziti/etc/controller/bootstrap.bash'"\
          "\n* Run 'systemctl enable --now ziti-controller.service'"\
          "\n"
}

dbFile() {
  if ! (( "${#}" )); then
    echo "ERROR: no config file path provided" >&2
    return 1
  fi
  local _config_file="${1}"
  awk -F: '/^db:/ {print $2}' "${_config_file}"|xargs realpath
}

# BEGIN

# discard debug unless this script is executed directly with DEBUG=1
# initialize a file descriptor for debug output
: "${DEBUG:=0}"
: "${VERBOSE:=${DEBUG}}"
if (( DEBUG )); then
  exec 3>&1
  set -o xtrace
else
  exec 3>/dev/null
fi


# set defaults
: "${ZITI_PKI_ROOT:=pki}"  # relative to working directory
: "${ZITI_CA_FILE:=root}"  # relative to ZITI_PKI_ROOT
: "${ZITI_INTERMEDIATE_FILE:=intermediate}"  # relative to ZITI_PKI_ROOT
: "${ZITI_SERVER_FILE:=server}"  # relative to intermediate CA "keys" and "certs" dirs
: "${ZITI_CLIENT_FILE:=client}"  # relative to intermediate CA "keys" and "certs" dirs
: "${ZITI_NETWORK_NAME:=ctrl}"  # basename of identity files
: "${ZITI_CTRL_DATABASE_FILE:=bbolt.db}"  # relative path to working directory


# run the bootstrap function if this script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then

  set -o errexit
  set -o nounset
  set -o pipefail

  if [[ $UID != 0 ]]; then
    echo "ERROR: must be run as root; when executed directly, this script prepares the working directory for"\
          "ziti-controller.service and generates a configuration" >&2
    exit 1
  fi

  export ZITI_HOME=/var/lib/ziti-controller
  DEFAULT_ADDR=localhost
  SVC_ENV_FILE=/opt/openziti/etc/controller/service.env
  BOOT_ENV_FILE=/opt/openziti/etc/controller/bootstrap.env
  SVC_FILE=/etc/systemd/system/ziti-controller.service.d/override.conf

  prepareWorkingDir "${ZITI_HOME}"
  loadEnvFiles                  # load lowest precedence vars from SVC_ENV_FILE then BOOT_ENV_FILE
  importZitiVars                # get ZITI_* vars from environment and set in BOOT_ENV_FILE
  loadEnvStdin                  # slurp answers from stdin if it's not a tty
  promptBootstrap               # prompt for ZITI_BOOTSTRAP if explicitly disabled (set and != true)
  promptCtrlAddress             # prompt for ZITI_CTRL_ADVERTISED_ADDRESS if not already set
  promptCtrlPort                # prompt for ZITI_CTRL_ADVERTISED_PORT if not already set
  promptBindAddress             # prompt for ZITI_CTRL_BIND_ADDRESS if not already set
  promptUser                    # prompt for ZITI_USER if not already set
  promptPwd                     # prompt for ZITI_PWD if not already set
  loadEnvFiles                  # reload env files to source new answers from prompts

  if ! (( $# ))
  then
    set -- "${ZITI_HOME}/config.yml"
  fi

  # suppress normal output during bootstrapping unless VERBOSE
  exec 4>&1; exec 1>/dev/null
  if (( VERBOSE )); then
    exec 1>&4
  fi
  if ZITI_BOOTSTRAP=true bootstrap "${@}"
  then
    finalizeWorkingDir "${ZITI_HOME}"
    setAnswer "ZITI_PWD=" "${SVC_ENV_FILE}" "${BOOT_ENV_FILE}"

    # successfully running this script directly means bootstrapping was enabled
    setBootstrapEnabled
  else
    echo "ERROR: something went wrong during bootstrapping; set DEBUG=1 for verbose output" >&2
  fi
fi
