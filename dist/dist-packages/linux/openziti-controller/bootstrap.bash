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
    echo "ERROR: ZITI_CTRL_ADVERTISED_ADDRESS must be set" >&2
    hintLinuxBootstrap "${PWD}"
    return 1
  fi

  # generate a root and intermediate unless explicitly disabled or an existing PKI dir was provided
  if [[ -z "${ZITI_CLUSTER_NODE_PKI:-}" && "${ZITI_BOOTSTRAP_CLUSTER}" == true ]]; then
    if [[ -z "${ZITI_CLUSTER_TRUST_DOMAIN:-}" ]]; then
      echo "ERROR: ZITI_CLUSTER_TRUST_DOMAIN must be set" >&2
      hintLinuxBootstrap "${PWD}"
      return 1
    fi

    if [[ "${ZITI_CA_FILE}" == "${ZITI_INTERMEDIATE_FILE}" ]]; then
      echo "ERROR: ZITI_CA_FILE and ZITI_INTERMEDIATE_FILE must be different" >&2
      return 1
    fi

    echo "DEBUG: generating new cluster PKI because ZITI_BOOTSTRAP_CLUSTER=true" >&3
    if [[ ! -s "${ZITI_CA_CERT}" ]]; then
      ziti pki create ca \
        --pki-root "${ZITI_PKI_ROOT}" \
        --ca-file "${ZITI_CA_FILE}" \
        --trust-domain "spiffe://${ZITI_CLUSTER_TRUST_DOMAIN}"
    fi

    if [[ ! -s "${ZITI_PKI_SIGNER_CERT}" && ! -s "${ZITI_PKI_SIGNER_KEY}" ]]; then
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
  elif [[ -z "${ZITI_CLUSTER_NODE_PKI:-}" && "${ZITI_BOOTSTRAP_CLUSTER}" == false ]]; then
    echo "DEBUG: not generating new cluster PKI because ZITI_BOOTSTRAP_CLUSTER=false and not installing new node PKI because ZITI_CLUSTER_NODE_PKI is not set" >&3
  else
    # install the provided intermediate signing cert in this node's PKI root
    echo "DEBUG: installing new node PKI from ZITI_CLUSTER_NODE_PKI=${ZITI_CLUSTER_NODE_PKI}" >&3
    cp -RT "${ZITI_CLUSTER_NODE_PKI}" "${ZITI_PKI_ROOT}"
  fi

  issueLeafCerts
}

issueLeafCerts() {
  #
  # create server and client keys
  #

  if [[ "${ZITI_SERVER_FILE}" == "${ZITI_CLIENT_FILE}" ]]; then
    echo "ERROR: ZITI_SERVER_FILE and ZITI_CLIENT_FILE must be different" >&2
    return 1
  fi

  if [[ -z "${ZITI_CLUSTER_NODE_NAME:-}" ]]; then
    echo "ERROR: ZITI_CLUSTER_NODE_NAME must be set" >&2
    hintLinuxBootstrap "${PWD}"
    return 1
  fi

  ZITI_PKI_CTRL_KEY="${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/keys/${ZITI_SERVER_FILE}.key"
  if ! [[ -s "$ZITI_PKI_CTRL_KEY" ]]; then
    ziti pki create key \
      --pki-root "${ZITI_PKI_ROOT}" \
      --ca-name "${ZITI_INTERMEDIATE_FILE}" \
      --key-file "${ZITI_SERVER_FILE}"
  fi

  #
  # create server and client certs
  #

  # server cert
  ZITI_PKI_CTRL_SERVER_CERT="${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/certs/${ZITI_SERVER_FILE}.chain.pem"
  if [[ "${ZITI_AUTO_RENEW_CERTS}" == true || ! -s "$ZITI_PKI_CTRL_SERVER_CERT" ]]; then

    _dns_sans="localhost"
    _ip_sans="127.0.0.1,::1"
    if [[ "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" =~ [0-9]{1,3}\.?{4} ]]; then
      _ip_sans+=",${ZITI_CTRL_ADVERTISED_ADDRESS}"
    else
      _dns_sans+=",${ZITI_CTRL_ADVERTISED_ADDRESS}"
    fi
    # server cert
    ziti pki create server \
      --pki-root "${ZITI_PKI_ROOT}" \
      --ca-name "${ZITI_INTERMEDIATE_FILE}" \
      --key-file "${ZITI_SERVER_FILE}" \
      --server-file "${ZITI_SERVER_FILE}" \
      --server-name "${ZITI_CLUSTER_NODE_NAME}" \
      --dns "${_dns_sans}" \
      --ip "${_ip_sans}" \
      --spiffe-id "controller/${ZITI_CLUSTER_NODE_NAME}" \
      --allow-overwrite >&3  # write to debug fd because this runs every startup
  fi

  # client cert
  #   use the server key for both client and server certs until "ziti create config controller" supports separate keys for
  #   each
  ZITI_PKI_CTRL_CERT="${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/certs/${ZITI_CLIENT_FILE}.chain.pem"
  if [[ "${ZITI_AUTO_RENEW_CERTS}" == true || ! -s "$ZITI_PKI_CTRL_CERT" ]]; then
    ziti pki create client \
      --pki-root "${ZITI_PKI_ROOT}" \
      --ca-name "${ZITI_INTERMEDIATE_FILE}" \
      --key-file "${ZITI_SERVER_FILE}" \
      --client-file "${ZITI_CLIENT_FILE}" \
      --client-name "${ZITI_CLUSTER_NODE_NAME}" \
      --spiffe-id "controller/${ZITI_CLUSTER_NODE_NAME}" \
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
    hintLinuxBootstrap "${PWD}"
    return 1
  else
    echo "DEBUG: ZITI_CTRL_ADVERTISED_ADDRESS is set to ${ZITI_CTRL_ADVERTISED_ADDRESS}" >&3
  fi

  # set the URI of the edge-client API (uses same TCP port); e.g., ztAPI: ctrl.ziti.example.com:1280
  export  ZITI_CTRL_EDGE_ADVERTISED_ADDRESS="${ZITI_CTRL_ADVERTISED_ADDRESS}" \
          ZITI_CTRL_EDGE_ADVERTISED_PORT="${ZITI_CTRL_ADVERTISED_PORT:=1280}"

  # export the vars that were assigned inside this script to set the path to the server and client certs and their common
  # private key, and the intermediate (signer) CA cert and key
  export  ZITI_PKI_EDGE_SERVER_CERT="${ZITI_PKI_CTRL_SERVER_CERT}" \
          ZITI_PKI_EDGE_CERT="${ZITI_PKI_CTRL_CERT}" \
          ZITI_PKI_EDGE_KEY="${ZITI_PKI_CTRL_KEY}" \
          ZITI_PKI_EDGE_CA="${ZITI_PKI_CTRL_CA}"

  # build config command
  local -a _command=(ziti create config controller --clustered --output "${_config_file}")

  # append args if ZITI_BOOTSTRAP_CONFIG_ARGS is not empty
  if [[ -n "${ZITI_BOOTSTRAP_CONFIG_ARGS:-}" ]]; then
    _command+=("${ZITI_BOOTSTRAP_CONFIG_ARGS}")
  fi

  # don't configure the console if explicitly disabled or if the location is not set or console files are missing
  if [[ "${ZITI_BOOTSTRAP_CONSOLE:-}" == true && -n "${ZITI_CONSOLE_LOCATION:-}" ]]; then
    if [[ ! -s "${ZITI_CONSOLE_LOCATION}/index.html" ]]; then
      echo "WARN: ${ZITI_CONSOLE_LOCATION}/index.html is missing; install 'openziti-console' to enable the console" >&2
    fi
  elif [[ "${ZITI_BOOTSTRAP_CONSOLE:-}" == false ]]; then
    unset ZITI_CONSOLE_LOCATION
    echo "DEBUG: ZITI_CONSOLE_LOCATION unset because ZITI_BOOTSTRAP_CONSOLE is false" >&3
  fi

  if [[ -s "${_config_file}" && "${1:-}" == --force ]]; then
    echo "INFO: recreating config file: ${_config_file}"
    mv --no-clobber "${_config_file}"{,".${ZITI_BOOTSTRAP_NOW}.old"}
  fi

  exportZitiVars
  # shellcheck disable=SC2068
  ${_command[@]}
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
  # assignments like ZITI_THING=abcd1234, one per line
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

# shellcheck disable=SC2120
loadEnvFiles() {
  if (( $#))
  then
    local -a _env_files=("${@}")
  else
    local -a _env_files=("${BOOT_ENV_FILE}" "${SVC_ENV_FILE}")
  fi
  for _env_file in "${_env_files[@]}"
  do
    if [[ -s "${_env_file}" ]]
    then
      # shellcheck disable=SC1090
      source "${_env_file}"
    else
      echo "WARN: missing env file '${_env_file}'" >&2
    fi 
  done
}

promptCtrlAddress() {
  if [[ -z "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" ]]; then
    if [[ -n "${ZITI_CLUSTER_NODE_NAME:-}" && -n "${ZITI_CLUSTER_TRUST_DOMAIN:-}" ]]; then
      ZITI_CTRL_ADVERTISED_ADDRESS="$(
        prompt "Enter DNS name of the controller [${ZITI_CLUSTER_NODE_NAME}.${ZITI_CLUSTER_TRUST_DOMAIN}]: " \
        || echo "${ZITI_CLUSTER_NODE_NAME}.${ZITI_CLUSTER_TRUST_DOMAIN}"
      )"
    else
      ZITI_CTRL_ADVERTISED_ADDRESS="$(prompt "Enter DNS name of the controller [required]: ")"
    fi
    if [[ -z "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" ]]; then
      echo "ERROR: missing required DNS name ZITI_CTRL_ADVERTISED_ADDRESS in ${BOOT_ENV_FILE}" >&2
      return 1
    else
      setAnswer "ZITI_CTRL_ADVERTISED_ADDRESS=${ZITI_CTRL_ADVERTISED_ADDRESS}" "${BOOT_ENV_FILE}"
    fi
  fi
}

promptClusterNodePki(){
  if [[ "${ZITI_BOOTSTRAP_CLUSTER:-}" == false && -z "${ZITI_CLUSTER_NODE_PKI:-}" ]]; then
    echo -e "\nThe PKI directory must contain:"\
            "\n\t${ZITI_CA_CERT}"\
            "\n\t${ZITI_PKI_SIGNER_CERT}"\
            "\n\t${ZITI_PKI_SIGNER_KEY}"\
            "\n"
    if ZITI_CLUSTER_NODE_PKI="$(prompt  "Enter the path to the new cluster node's PKI directory: " )"; then
      setAnswer "ZITI_CLUSTER_NODE_PKI=${ZITI_CLUSTER_NODE_PKI}" "${BOOT_ENV_FILE}"
    else
      echo "ERROR: missing ZITI_CLUSTER_NODE_PKI in ${BOOT_ENV_FILE}; required for joining an existing cluster" >&2
      return 1
    fi
  fi
}

promptBootstrapCluster(){
  if [[ -z "${ZITI_BOOTSTRAP_CLUSTER:-}" ]]; then
    ZITI_BOOTSTRAP_CLUSTER="$(prompt 'Create a new cluster (NO if joining a cluster) [Y/n]: ' || echo 'true')"
    if [[ "${ZITI_BOOTSTRAP_CLUSTER}" =~ ^([yY]([eE][sS])?|[tT]([rR][uU][eE])?)$ ]]; then
      ZITI_BOOTSTRAP_CLUSTER=true
    elif [[ "${ZITI_BOOTSTRAP_CLUSTER}" =~ ^([nN][oO]?|[fF]([aA][lL][sS][eE])?)$ ]]; then
      ZITI_BOOTSTRAP_CLUSTER=false
    fi
    setAnswer "ZITI_BOOTSTRAP_CLUSTER=${ZITI_BOOTSTRAP_CLUSTER}" "${BOOT_ENV_FILE}"
  fi
}

promptClusterNodeName(){
  if [[ -z "${ZITI_CLUSTER_NODE_NAME:-}" ]]; then
    # if the address contains at least two dots then use the first part as the default node name
    if [[ -n "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" && "${ZITI_CTRL_ADVERTISED_ADDRESS}" =~ .+\..+\..+ ]]; then
      ZITI_CLUSTER_NODE_NAME="$(
        prompt "Enter the unique name for this node in the cluster [${ZITI_CTRL_ADVERTISED_ADDRESS%%.*}]: " \
        || echo "${ZITI_CTRL_ADVERTISED_ADDRESS%%.*}"
      )"
    else
      if ! ZITI_CLUSTER_NODE_NAME="$(prompt "Enter the unique name for this node in the cluster [required]: ")"; then
        echo "ERROR: missing required ZITI_CLUSTER_NODE_NAME in ${BOOT_ENV_FILE}" >&2
        return 1
      fi
    fi
    if [[ -n "${ZITI_CLUSTER_NODE_NAME:-}" ]]; then
      setAnswer "ZITI_CLUSTER_NODE_NAME=${ZITI_CLUSTER_NODE_NAME}" "${BOOT_ENV_FILE}"
    else
      echo "ERROR: missing required ZITI_CLUSTER_NODE_NAME in ${BOOT_ENV_FILE}" >&2
      return 1
    fi
  fi
}

promptClusterTrustDomain() {
  if [[ "${ZITI_BOOTSTRAP_CLUSTER:-}" == true && -z "${ZITI_CLUSTER_TRUST_DOMAIN:-}" ]]; then
    local _prompt="Enter the trust domain shared by all nodes in the cluster"
    # if the address contains at least two dots then use everything after the first dot as the default trust domain
    if [[ "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" =~ .+\..+\..+ ]]; then
      ZITI_CLUSTER_TRUST_DOMAIN="$(
        prompt "${_prompt} [${ZITI_CTRL_ADVERTISED_ADDRESS#*.}]: " \
        || echo "${ZITI_CTRL_ADVERTISED_ADDRESS#*.}"
      )"
    else
      ZITI_CLUSTER_TRUST_DOMAIN="$(prompt "${_prompt} [required]: ")" || true
    fi
    if [[ -z "${ZITI_CLUSTER_TRUST_DOMAIN:-}" ]]; then
      echo "ERROR: missing required ZITI_CLUSTER_TRUST_DOMAIN in ${BOOT_ENV_FILE}" >&2
      return 1
    else
      setAnswer "ZITI_CLUSTER_TRUST_DOMAIN=${ZITI_CLUSTER_TRUST_DOMAIN}" "${BOOT_ENV_FILE}"
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
    echo "ERROR: setAnswer() requires at least two arguments, e.g., setAnswer 'ZITI_THING=abcd1234' ./some1.env ./some2.env" >&2
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
  # make ziti vars available in forks
  for line in $(set | grep -e "^ZITI_" | sort); do
    # shellcheck disable=SC2013
    export "${line%=*}"
  done
}

bootstrap() {

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

  _ctrl_data_dir="$(dataDir "${_ctrl_config_file}")"
  if [[ -d "${_ctrl_data_dir}" ]]; then
    echo "DEBUG: database directory exists in $(realpath "${_ctrl_data_dir}")" >&3
  else
    echo "DEBUG: creating database directory $(realpath "${_ctrl_data_dir}")" >&3
    # shellcheck disable=SC2174
    mkdir -pm0700 "${_ctrl_data_dir}"
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

hintLinuxBootstrap() {

  local _work_dir="${1:-${PWD}}"

  echo -e "\nProvide a configuration in '${_work_dir}' or generate with:"\
          "\n* Set vars in'/opt/openziti/etc/controller/bootstrap.env'"\
          "\n* Run '/opt/openziti/etc/controller/bootstrap.bash'"\
          "\n* Run 'systemctl enable --now ziti-controller.service'"\
          "\n"
}

dataDir() {
  if ! (( "${#}" )); then
    echo "ERROR: no config file path provided" >&2
    return 1
  fi
  local _config_file="${1}"
  awk -F: '/^[[:space:]]+dataDir:/ {print $2}' "${_config_file}"|xargs realpath
}

exitHandler() {
  echo "WARN: set VERBOSE=1 or DEBUG=1 for more output" >&2
  if [[ -s "${INFO_LOG_FILE:-}" || -s "${DEBUG_LOG_FILE:-}" ]]; then
    cat "${INFO_LOG_FILE:-/dev/null}" "${DEBUG_LOG_FILE:-/dev/null}" >> "${ZITI_BOOTSTRAP_LOG_FILE}"
    echo "WARN: see output in '${ZITI_BOOTSTRAP_LOG_FILE}'" >&2
  fi
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
  exec 3>>"${DEBUG_LOG_FILE:=$(mktemp)}"
fi

trap exitHandler EXIT SIGINT SIGTERM

# set defaults
: "${ZITI_PKI_ROOT:=pki}"  # relative to working directory
: "${ZITI_CA_FILE:=root}"  # relative to ZITI_PKI_ROOT
: "${ZITI_INTERMEDIATE_FILE:=intermediate}"  # relative to ZITI_PKI_ROOT
: "${ZITI_SERVER_FILE:=server}"  # relative to intermediate CA "keys" and "certs" dirs
: "${ZITI_CLIENT_FILE:=client}"  # relative to intermediate CA "keys" and "certs" dirs
: "${ZITI_NETWORK_NAME:=ctrl}"  # basename of identity files
: "${ZITI_CTRL_BIND_ADDRESS:=0.0.0.0}"  # the interface address on which to listen
: "${ZITI_BOOTSTRAP_LOG_FILE:=$(mktemp)}"  # where the exit handler should concatenate verbose and debug messages
ZITI_CA_CERT="${ZITI_PKI_ROOT}/${ZITI_CA_FILE}/certs/${ZITI_CA_FILE}.cert"
ZITI_PKI_SIGNER_CERT="${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/certs/${ZITI_INTERMEDIATE_FILE}.cert"
ZITI_PKI_SIGNER_KEY="${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/keys/${ZITI_INTERMEDIATE_FILE}.key"
ZITI_PKI_CTRL_SERVER_CERT="${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/certs/${ZITI_SERVER_FILE}.chain.pem"
ZITI_PKI_CTRL_CERT="${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/certs/${ZITI_CLIENT_FILE}.chain.pem"
ZITI_PKI_CTRL_KEY="${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/keys/${ZITI_SERVER_FILE}.key"
ZITI_PKI_CTRL_CA="${ZITI_PKI_ROOT}/${ZITI_CA_FILE}/certs/${ZITI_CA_FILE}.cert"
ZITI_BOOTSTRAP_NOW="$(date --utc --iso-8601=seconds)"


# if sourced then only define vars and functions and change working directory; else if exec'd then run bootstrap()
if ! [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then

  # ensure ZITI_HOME is working dir to allow paths to be relative or absolute
  cd "${ZITI_HOME:=${PWD}}" || {
    echo "ERROR: failed to cd to '${ZITI_HOME}'" >&2
    exit 1
  }

else

  set -o errexit
  set -o nounset
  set -o pipefail

  export ZITI_HOME=/var/lib/private/ziti-controller
  SVC_ENV_FILE=/opt/openziti/etc/controller/service.env
  BOOT_ENV_FILE=/opt/openziti/etc/controller/bootstrap.env
  SVC_FILE=/etc/systemd/system/ziti-controller.service.d/override.conf
  : "${ZITI_CONSOLE_LOCATION:=/opt/openziti/share/console}"

  if [[ "${1:-}" =~ ^[-] ]]
  then
    echo -e "\nUsage:"\
            "\n\t$0 [CONFIG_FILE]"\
            "\n" \
            "\nOPTIONS" \
            "\n" \
            "\nVERBOSE=1\tprint INFO" \
            "\nDEBUG=1\t\tprint DEBUG" \
            "\n" >&2
    hintLinuxBootstrap "${ZITI_HOME}"
    exit 1
  elif (( $# ))
  then
    set -- "${ZITI_HOME}/$(basename "$1")"
  else
    set -- "${ZITI_HOME}/config.yml"
  fi
  echo "DEBUG: using config file: $*" >&3

  if [[ $UID != 0 ]]; then
    echo "ERROR: must be run as root" >&2
    exit 1
  fi

  prepareWorkingDir "${ZITI_HOME}"
  loadEnvFiles                  # load lowest precedence vars from SVC_ENV_FILE then BOOT_ENV_FILE
  importZitiVars                # get ZITI_* vars from environment and set in BOOT_ENV_FILE
  loadEnvStdin                  # slurp answers from stdin if it's not a tty
  promptBootstrap               # prompt for ZITI_BOOTSTRAP if explicitly disabled (set and != true)
  promptBootstrapCluster        # prompt for new cluster or existing PKI
  promptClusterNodeName         # prompt for ZITI_CLUSTER_NODE_NAME if not already set
  promptClusterTrustDomain      # prompt for ZITI_CLUSTER_TRUST_DOMAIN if not already set
  promptClusterNodePki          # prompt for ZITI_CLUSTER_NODE_PKI if not already set and not bootstrapping a new cluster
  promptCtrlAddress             # prompt for ZITI_CTRL_ADVERTISED_ADDRESS if not already set
  promptCtrlPort                # prompt for ZITI_CTRL_ADVERTISED_PORT if not already set
  loadEnvFiles                  # reload env files to source new answers from prompts

  # suppress normal output during bootstrapping unless VERBOSE
  exec 4>&1; exec 1>>"${INFO_LOG_FILE:=$(mktemp)}"
  if (( VERBOSE )); then
    exec 1>&4
  fi
  
  # run bootstrap(), set filemodes
  if bootstrap "${@}"
  then
    echo "DEBUG: bootstrap complete" >&3
    finalizeWorkingDir "${ZITI_HOME}"

    # successfully running this script directly means bootstrapping was enabled
    setAnswer "ZITI_BOOTSTRAP=true" "${SVC_ENV_FILE}"
    # if verbose then this was already done earlier, else allow stdout now to announce completion
    if ! (( VERBOSE )); then
      exec 1>&4
    fi
    echo -e "INFO: bootstrap completed successfully and will not run again."\
            "Adjust ${ZITI_HOME}/config.yml to suit." >&2
    trap - EXIT  # remove exit trap
  else
    echo "ERROR: something went wrong during bootstrapping" >&2
  fi
fi
