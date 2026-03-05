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
    # install the root CA from the provided PKI directory and create a new intermediate for this node
    echo "DEBUG: installing root CA from ZITI_CLUSTER_NODE_PKI=${ZITI_CLUSTER_NODE_PKI}" >&3
    local _src_ca_cert="${ZITI_CLUSTER_NODE_PKI}/${ZITI_CA_FILE}/certs/${ZITI_CA_FILE}.cert"
    local _src_ca_key="${ZITI_CLUSTER_NODE_PKI}/${ZITI_CA_FILE}/keys/${ZITI_CA_FILE}.key"
    if [[ ! -s "${_src_ca_cert}" ]]; then
      echo "ERROR: root CA cert not found: ${_src_ca_cert}" >&2
      return 1
    fi
    if [[ ! -s "${_src_ca_key}" ]]; then
      echo "ERROR: root CA key not found: ${_src_ca_key}" >&2
      return 1
    fi
    local _ca_dir="${ZITI_PKI_ROOT}/${ZITI_CA_FILE}"
    mkdir -p "${_ca_dir}/certs" "${_ca_dir}/keys" "${_ca_dir}/crls"
    cp "${_src_ca_cert}" "${_ca_dir}/certs/${ZITI_CA_FILE}.cert"
    cp "${_src_ca_key}" "${_ca_dir}/keys/${ZITI_CA_FILE}.key"
    # initialize CA index files required by ziti pki create intermediate
    [[ -f "${_ca_dir}/index.txt" ]]      || touch "${_ca_dir}/index.txt"
    [[ -f "${_ca_dir}/index.txt.attr" ]]  || touch "${_ca_dir}/index.txt.attr"
    [[ -f "${_ca_dir}/serial" ]]          || echo "01" > "${_ca_dir}/serial"
    [[ -f "${_ca_dir}/crlnumber" ]]       || echo "01" > "${_ca_dir}/crlnumber"
    if [[ ! -s "${ZITI_PKI_SIGNER_CERT}" && ! -s "${ZITI_PKI_SIGNER_KEY}" ]]; then
      ziti pki create intermediate \
        --pki-root "${ZITI_PKI_ROOT}" \
        --ca-name "${ZITI_CA_FILE}" \
        --intermediate-file "${ZITI_INTERMEDIATE_FILE}"
    elif [[ ! -s "${ZITI_PKI_SIGNER_CERT}" || ! -s "${ZITI_PKI_SIGNER_KEY}" ]]; then
      echo "ERROR: ${ZITI_PKI_SIGNER_CERT} and ${ZITI_PKI_SIGNER_KEY} must both exist or neither exist as non-empty files" >&2
      return 1
    else
      echo "INFO: intermediate CA exists in $(realpath "${ZITI_PKI_SIGNER_CERT}")"
    fi
    # NOTE: the root CA key is kept on disk for edge enrollment signer issuance and renewal.
    # It is not used during daily operations and can be secured offline separately.
  fi

  # Create the shared private key for server + client leaf certs.
  # The leaf certs themselves are issued by the systemd timer service.
  makePrivateKey
}

makePrivateKey() {
  #
  # create the server/client private key (shared by both leaf certs)
  #

  ZITI_PKI_CTRL_KEY="${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/keys/${ZITI_SERVER_FILE}.key"
  if ! [[ -s "$ZITI_PKI_CTRL_KEY" ]]; then
    ziti pki create key \
      --pki-root "${ZITI_PKI_ROOT}" \
      --ca-name "${ZITI_INTERMEDIATE_FILE}" \
      --key-file "${ZITI_SERVER_FILE}"
  fi
}

issueLeafCerts() {
  #
  # Issue server and client leaf certificates from the intermediate CA.
  # On Linux, this is called by the systemd cert renewal timer, not directly.
  # On Docker, bootstrap() calls this for renewal when ZITI_AUTO_RENEW_CERTS=true.
  #

  if [[ "${ZITI_SERVER_FILE}" == "${ZITI_CLIENT_FILE}" ]]; then
    echo "ERROR: ZITI_SERVER_FILE and ZITI_CLIENT_FILE must be different" >&2
    return 1
  fi

  # node name is required for clustered mode SPIFFE IDs; fall back to ZITI_NETWORK_NAME for standalone
  if [[ -z "${ZITI_CLUSTER_NODE_NAME:-}" && "${ZITI_BOOTSTRAP_CLUSTER:-}" == true ]]; then
    echo "ERROR: ZITI_CLUSTER_NODE_NAME must be set for clustered mode" >&2
    hintLinuxBootstrap "${PWD}"
    return 1
  fi
  local _cert_name="${ZITI_CLUSTER_NODE_NAME:-${ZITI_NETWORK_NAME}}"

  # ensure private key exists
  makePrivateKey

  # server cert
  ZITI_PKI_CTRL_SERVER_CERT="${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/certs/${ZITI_SERVER_FILE}.chain.pem"
  if [[ "${ZITI_AUTO_RENEW_CERTS}" == true || ! -s "$ZITI_PKI_CTRL_SERVER_CERT" ]]; then
    local _dns_sans="localhost"
    local _ip_sans="127.0.0.1,::1"
    if [[ "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" =~ ^([0-9]{1,3}\.?){4} ]]; then
      _ip_sans+=",${ZITI_CTRL_ADVERTISED_ADDRESS}"
    else
      _dns_sans+=",${ZITI_CTRL_ADVERTISED_ADDRESS}"
    fi
    local -a _server_cmd=(ziti pki create server
      --pki-root "${ZITI_PKI_ROOT}"
      --ca-name "${ZITI_INTERMEDIATE_FILE}"
      --key-file "${ZITI_SERVER_FILE}"
      --server-file "${ZITI_SERVER_FILE}"
      --server-name "${_cert_name}"
      --dns "${_dns_sans}"
      --ip "${_ip_sans}"
      --allow-overwrite
      --spiffe-id "controller/${ZITI_CLUSTER_NODE_NAME}")
    "${_server_cmd[@]}" >&3
  fi

  # client cert (shares the server key because "ziti create config controller" expects a single key for both)
  ZITI_PKI_CTRL_CERT="${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/certs/${ZITI_CLIENT_FILE}.chain.pem"
  if [[ "${ZITI_AUTO_RENEW_CERTS}" == true || ! -s "$ZITI_PKI_CTRL_CERT" ]]; then
    local -a _client_cmd=(ziti pki create client
      --pki-root "${ZITI_PKI_ROOT}"
      --ca-name "${ZITI_INTERMEDIATE_FILE}"
      --key-file "${ZITI_SERVER_FILE}"
      --client-file "${ZITI_CLIENT_FILE}"
      --client-name "${_cert_name}"
      --allow-overwrite
      --spiffe-id "controller/${ZITI_CLUSTER_NODE_NAME}")
    "${_client_cmd[@]}" >&3
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

stashZitiEnv() {
  declare -gA _ziti_env_saved=()
  while IFS='=' read -r _key _value; do
    _ziti_env_saved["${_key}"]="${_value}"
  done < <(env | grep '^ZITI_')
}

restoreZitiEnv() {
  for _key in "${!_ziti_env_saved[@]}"; do
    if [[ -n "${_ziti_env_saved[${_key}]}" ]]; then
      printf -v "${_key}" '%s' "${_ziti_env_saved[${_key}]}"
    fi
  done
  unset _ziti_env_saved _key _value
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

# shellcheck disable=SC2120
loadEnvFiles() {
  if (( $#))
  then
    local -a _env_files=("${@}")
  else
    local -a _env_files=("${SVC_ENV_FILE}")
  fi
  for _env_file in "${_env_files[@]}"
  do
    if [[ -s "${_env_file}" ]]
    then
      # shellcheck disable=SC1090
      source "${_env_file}"
    else
      echo "DEBUG: env file not found or empty: '${_env_file}'" >&3
    fi
  done
}

promptCtrlAddress() {
  if [[ -z "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" ]]; then
    if isInteractive; then
      while [[ -z "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" ]]; do
        ZITI_CTRL_ADVERTISED_ADDRESS="$(prompt "Enter DNS name of the controller (required): ")" || true
      done
    fi
    if [[ -z "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" ]]; then
      echo "ERROR: ZITI_CTRL_ADVERTISED_ADDRESS is required" >&2
      return 1
    else
      setAnswer "ZITI_CTRL_ADVERTISED_ADDRESS=${ZITI_CTRL_ADVERTISED_ADDRESS}" "${BOOT_ENV_FILE}"
    fi
  fi
}

promptClusterNodePki(){
  if [[ "${ZITI_BOOTSTRAP_CLUSTER:-}" == false && -z "${ZITI_CLUSTER_NODE_PKI:-}" ]]; then
    echo -e "\nThe PKI directory must contain the root CA cert and key:"\
            "\n\t${ZITI_CA_FILE}/certs/${ZITI_CA_FILE}.cert"\
            "\n\t${ZITI_CA_FILE}/keys/${ZITI_CA_FILE}.key"\
            "\n"
    if isInteractive; then
      while [[ -z "${ZITI_CLUSTER_NODE_PKI:-}" ]]; do
        ZITI_CLUSTER_NODE_PKI="$(prompt "Enter the path to the existing cluster's PKI directory (required): ")" || true
      done
      setAnswer "ZITI_CLUSTER_NODE_PKI=${ZITI_CLUSTER_NODE_PKI}" "${BOOT_ENV_FILE}"
    else
      echo "ERROR: ZITI_CLUSTER_NODE_PKI is required for joining an existing cluster" >&2
      return 1
    fi
  fi
}

promptBootstrapCluster(){
  # Prompt if ZITI_BOOTSTRAP_CLUSTER is unset and database bootstrapping is enabled
  if [[ -z "${ZITI_BOOTSTRAP_CLUSTER:-}" && "${ZITI_BOOTSTRAP_DATABASE:-}" == true ]]; then
    local _joining
    _joining="$(prompt 'Are you joining an existing cluster? [y/N]: ' || echo 'false')"
    if [[ "${_joining}" =~ ^([yY]([eE][sS])?|[tT]([rR][uU][eE])?)$ ]]; then
      ZITI_BOOTSTRAP_CLUSTER=false
    else
      ZITI_BOOTSTRAP_CLUSTER=true
    fi
    setAnswer "ZITI_BOOTSTRAP_CLUSTER=${ZITI_BOOTSTRAP_CLUSTER}" "${BOOT_ENV_FILE}"
  fi
}

promptClusterNodeName(){
  # Prompt if ZITI_CLUSTER_NODE_NAME is unset and database bootstrapping is enabled
  if [[ -z "${ZITI_CLUSTER_NODE_NAME:-}" && "${ZITI_BOOTSTRAP_DATABASE:-}" == true ]]; then
    # derive default from controller address: first label of FQDN (requires at least 2 dots)
    local _default=""
    if [[ -n "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" && "${ZITI_CTRL_ADVERTISED_ADDRESS}" =~ .+\..+\..+ ]]; then
      _default="${ZITI_CTRL_ADVERTISED_ADDRESS%%.*}"
    fi
    if [[ -n "${_default}" ]]; then
      ZITI_CLUSTER_NODE_NAME="$(
        prompt "Enter the unique name for this node in the cluster [${_default}]: " \
        || echo "${_default}"
      )"
    elif isInteractive; then
      while [[ -z "${ZITI_CLUSTER_NODE_NAME:-}" ]]; do
        ZITI_CLUSTER_NODE_NAME="$(prompt "Enter the unique name for this node in the cluster (required): ")" || true
      done
    fi
    if [[ -n "${ZITI_CLUSTER_NODE_NAME:-}" ]]; then
      setAnswer "ZITI_CLUSTER_NODE_NAME=${ZITI_CLUSTER_NODE_NAME}" "${BOOT_ENV_FILE}"
    else
      echo "ERROR: ZITI_CLUSTER_NODE_NAME is required" >&2
      return 1
    fi
  fi
}

promptClusterTrustDomain() {
  # Prompt if creating a new cluster and trust domain is unset
  if [[ "${ZITI_BOOTSTRAP_CLUSTER:-}" == true && -z "${ZITI_CLUSTER_TRUST_DOMAIN:-}" ]]; then
    local _prompt="Enter the trust domain shared by all nodes in the cluster"
    # derive default from controller address: everything after the first label
    local _default=""
    if [[ "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" =~ .+\..+\..+ ]]; then
      _default="${ZITI_CTRL_ADVERTISED_ADDRESS#*.}"
    fi
    if [[ -n "${_default}" ]]; then
      ZITI_CLUSTER_TRUST_DOMAIN="$(
        prompt "${_prompt} [${_default}]: " \
        || echo "${_default}"
      )"
    elif isInteractive; then
      while [[ -z "${ZITI_CLUSTER_TRUST_DOMAIN:-}" ]]; do
        ZITI_CLUSTER_TRUST_DOMAIN="$(prompt "${_prompt} (required): ")" || true
      done
    fi
    if [[ -z "${ZITI_CLUSTER_TRUST_DOMAIN:-}" ]]; then
      echo "ERROR: ZITI_CLUSTER_TRUST_DOMAIN is required" >&2
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
        setAnswer "ZITI_BOOTSTRAP=${ZITI_BOOTSTRAP}" "${SVC_ENV_FILE}" "${BOOT_ENV_FILE}"
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

promptUser() {
  # Prompt for admin username if database bootstrapping is enabled and not already set
  if [[ -z "${ZITI_USER:-}" && "${ZITI_BOOTSTRAP_DATABASE:-}" == true ]]; then
    if ZITI_USER="$(prompt 'Enter the name of the default user [admin]: ' || echo 'admin')"; then
      setAnswer "ZITI_USER=${ZITI_USER}" "${BOOT_ENV_FILE}"
    fi
  fi
}

promptPassword() {
  # Joiners (ZITI_CLUSTER_NODE_PKI is set) don't need a password — they join
  # an existing cluster where the admin user was already created.
  if [[ -n "${ZITI_CLUSTER_NODE_PKI:-}" ]]; then
    return 0
  fi
  # prompt for password if database bootstrapping enabled and password not already set
  if [[ "${ZITI_BOOTSTRAP_DATABASE:-}" == true && -z "${ZITI_PWD:-}" ]]; then
    if isInteractive; then
      local _gen_answer
      _gen_answer="$(prompt "Generate a random password for '${ZITI_USER:-admin}'? [Y/n]: " || echo "y")"
      if [[ "${_gen_answer}" =~ ^([nN][oO]?)$ ]]; then
        # manual entry — re-prompt until non-empty
        while [[ -z "${ZITI_PWD:-}" ]]; do
          ZITI_PWD="$(prompt "Enter password for '${ZITI_USER:-admin}' (required): ")" || true
        done
      else
        ZITI_PWD=$(head -c128 /dev/urandom | LC_ALL=C tr -dc 'A-Za-z0-9!@#$%^*_+~' | cut -c 1-22)
        echo "Password: ${ZITI_PWD}"
        echo "NOTE: this password is not saved anywhere — record it now."
      fi
      setAnswer "ZITI_PWD=${ZITI_PWD}" "${BOOT_ENV_FILE}"
    else
      echo "ERROR: ZITI_PWD is required" >&2
      return 1
    fi
  fi
}

promptConsole() {
  if isInteractive; then
    local _console_answer
    _console_answer="$(prompt "Configure the OpenZiti Console (web UI)? [Y/n]: " || echo "y")"
    if [[ "${_console_answer}" =~ ^([nN][oO]?)$ ]]; then
      ZITI_BOOTSTRAP_CONSOLE=false
    else
      ZITI_BOOTSTRAP_CONSOLE=true
      if ! [[ -s "${ZITI_CONSOLE_LOCATION:-/opt/openziti/share/console}/index.html" ]]; then
        echo "WARN: install 'openziti-console' package to provide console assets" >&2
      fi
    fi
  fi
  # Non-interactive: honor env var / answer file / service.env default
  : "${ZITI_BOOTSTRAP_CONSOLE:=true}"
  setAnswer "ZITI_BOOTSTRAP_CONSOLE=${ZITI_BOOTSTRAP_CONSOLE}" "${BOOT_ENV_FILE}"
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
  # Inherit ZITI_* vars from the environment. Feature flags (keys already in
  # service.env) update there; all others go to the temp boot env file.
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

# Bootstrap the controller: generate PKI, config, and database directory.
#
# Docker: entrypoint.bash calls this for both initial setup and restarts.
#         issueLeafCerts() is called directly for Docker cert renewal.
# Linux:  bootstrap.bash calls this for initial setup only.
#         The systemd cert renewal timer handles leaf cert issuance + renewal.
bootstrap() {

  if [[ -n "${1:-}" ]]; then
    local _ctrl_config_file="${1}"
    echo "DEBUG: using config: $(realpath "${_ctrl_config_file}")" >&3
  else
    echo "ERROR: no config file path provided" >&2
    return 1
  fi

  # make PKI unless explicitly disabled
  if [[ "${ZITI_BOOTSTRAP_PKI}" == true ]]; then
    if ! [[ -s "${_ctrl_config_file}" ]]; then
      # First bootstrap: create root + intermediate CAs
      makePki
      if [[ -d /run/systemd/system ]]; then
        # Linux: the systemd timer handles both initial issuance and renewal.
        # installCertRenewalTimer() (called later) runs the oneshot immediately.
        if isInteractive; then
          echo "NOTE: leaf certs are renewed monthly by ziti-controller-cert-renewal.timer" >&2
        fi
      else
        # Docker: no systemd, issue leaf certs directly
        issueLeafCerts
      fi
    elif [[ "${ZITI_AUTO_RENEW_CERTS:-}" == true && ! -d /run/systemd/system ]]; then
      # Docker restart with existing config: renew leaf certs directly.
      # On Linux, the systemd timer handles renewal independently.
      issueLeafCerts
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
  if [[ -z "${_ctrl_data_dir}" ]]; then
    echo "DEBUG: dataDir not defined in ${_ctrl_config_file}, skipping database directory creation" >&3
  elif [[ -d "${_ctrl_data_dir}" ]]; then
    echo "DEBUG: database directory exists in $(realpath "${_ctrl_data_dir}")" >&3
  else
    echo "DEBUG: creating database directory $(realpath "${_ctrl_data_dir}")" >&3
    # shellcheck disable=SC2174
    mkdir -pm0700 "${_ctrl_data_dir}"
  fi

}

# Initialize the cluster with a default admin user. This is idempotent - if the cluster is already
# initialized, the command returns "already initialized" which we treat as success.
#
# This function is portable:
# - Linux server: called after starting systemd service, targets the controller by PID
# - Docker: called after controller is running in background, uses ziti agent directly
#
# Arguments:
#   $1 - (optional) controller PID (Linux path) or empty for direct agent call (Docker)
#
# Environment:
#   ZITI_BOOTSTRAP_CLUSTER - must be "true" to attempt initialization
#   ZITI_BOOTSTRAP_DATABASE - must be "true" to attempt initialization (alias for clarity)
#   ZITI_PWD - required password for the default admin
#   ZITI_USER - username for default admin (default: "admin")
#   ZITI_USER_NAME - display name for default admin (default: "Default Admin")
#
clusterInit() {
  # Only run for new cluster leaders — joiners (ZITI_CLUSTER_NODE_PKI set) don't
  # create admin users; they join an existing cluster where admin already exists.
  if [[ -n "${ZITI_CLUSTER_NODE_PKI:-}" ]]; then
    echo "DEBUG: skipping cluster init (joiner — ZITI_CLUSTER_NODE_PKI is set)" >&3
    return 0
  fi
  if [[ "${ZITI_BOOTSTRAP_CLUSTER:-}" != true && "${ZITI_BOOTSTRAP_DATABASE:-}" != true ]]; then
    echo "DEBUG: skipping cluster init (ZITI_BOOTSTRAP_CLUSTER=${ZITI_BOOTSTRAP_CLUSTER:-}, ZITI_BOOTSTRAP_DATABASE=${ZITI_BOOTSTRAP_DATABASE:-})" >&3
    return 0
  fi

  # Require password for initialization
  if [[ -z "${ZITI_PWD:-}" ]]; then
    echo "ERROR: ZITI_PWD must be set to initialize the cluster with a default admin" >&2
    return 1
  fi

  : "${ZITI_USER:=admin}"
  : "${ZITI_USER_NAME:=Default Admin}"

  local _pid="${1:-}"
  local _output
  local _rc
  local _attempts=10
  local _delay=3

  echo "INFO: initializing cluster with default admin '${ZITI_USER}'"

  while (( _attempts-- )); do
    set +o errexit
    if [[ -n "${_pid}" ]]; then
      _output="$(ziti agent cluster init --pid "${_pid}" "${ZITI_USER}" "${ZITI_PWD}" "${ZITI_USER_NAME}" 2>&1)"
    else
      _output="$(ziti agent cluster init "${ZITI_USER}" "${ZITI_PWD}" "${ZITI_USER_NAME}" 2>&1)"
    fi
    _rc=$?
    set -o errexit

    # Check result - treat "already initialized" as success for idempotency
    if (( _rc == 0 )); then
      echo "INFO: cluster initialized successfully"
      return 0
    elif [[ "${_output}" == *"already initialized"* ]]; then
      echo "INFO: cluster was already initialized"
      return 0
    fi

    # Retry on timeout — raft may still be initializing after the agent socket is ready
    if [[ "${_output}" == *"timeout"* || "${_output}" == *"deadline exceeded"* ]] && (( _attempts > 0 )); then
      echo "DEBUG: cluster init not ready, retrying in ${_delay}s (${_attempts} attempts left)" >&3
      sleep "${_delay}"
      continue
    fi

    echo "ERROR: cluster initialization failed (exit ${_rc}): ${_output}" >&2
    return "${_rc}"
  done

  echo "ERROR: cluster initialization timed out after all retries" >&2
  return 1
}

# Wait for the controller agent to become available
# Arguments:
#   $1 - (optional) controller PID, or empty for direct call
#   $2 - (optional) max attempts (default: 30)
#   $3 - (optional) delay between attempts in seconds (default: 1)
waitForAgent() {
  local _pid="${1:-}"
  local _attempts="${2:-30}"
  local _delay="${3:-1}"

  echo "DEBUG: waiting for controller agent to become available (max ${_attempts} attempts)" >&3

  while (( _attempts-- )); do
    local _rc
    set +o errexit
    if [[ -n "${_pid}" ]]; then
      ziti agent stats --pid "${_pid}" &>/dev/null
    else
      ziti agent stats &>/dev/null
    fi
    _rc=$?
    set -o errexit

    if (( _rc == 0 )); then
      echo "DEBUG: controller agent is available" >&3
      return 0
    fi
    sleep "${_delay}"
  done

  echo "ERROR: controller agent did not become available" >&2
  return 1
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

  chown -R ziti-controller:ziti-controller "${_config_dir}/"
  chmod -R u=rwX,go-rwx "${_config_dir}/"
}

hintLinuxBootstrap() {

  local _work_dir="${1:-${PWD}}"

  echo -e "\nProvide a configuration in '${_work_dir}' or generate with:"\
          "\n  /opt/openziti/etc/controller/bootstrap.bash [/path/to/answers.env]"\
          "\n"
}

# Generate and install a systemd timer+service pair for automatic leaf cert
# renewal.  The service unit contains the fully resolved `ziti pki create`
# commands — no env file indirection, no sourcing of bootstrap.bash.
#
# Leaf certs (server + client) default to 365 days.  The timer renews monthly
# (with up to 1 week of jitter) so certs are always fresh with ample buffer.
#
# Arguments:  none (reads current ZITI_* environment to render concrete paths)
# Requires:   systemd, ZITI_BOOTSTRAP_PKI=true
installCertRenewalTimer() {
  if [[ ! -d /run/systemd/system ]]; then
    echo "DEBUG: systemd not available, skipping cert renewal timer" >&3
    return 0
  fi

  local _svc_unit="/etc/systemd/system/ziti-controller-cert-renewal.service"
  local _timer_unit="/etc/systemd/system/ziti-controller-cert-renewal.timer"
  local _svc_user="ziti-controller"
  local _svc_group="ziti-controller"

  # Resolve all paths to absolute values at generation time
  local _pki_root
  _pki_root="$(cd "${ZITI_HOME}" && realpath "${ZITI_PKI_ROOT}")"
  local _cert_name="${ZITI_CLUSTER_NODE_NAME:-${ZITI_NETWORK_NAME}}"

  # Compute SANs exactly as issueLeafCerts() does
  local _dns_sans="localhost"
  local _ip_sans="127.0.0.1,::1"
  if [[ "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" =~ ^([0-9]{1,3}\.?){4} ]]; then
    _ip_sans+=",${ZITI_CTRL_ADVERTISED_ADDRESS}"
  else
    _dns_sans+=",${ZITI_CTRL_ADVERTISED_ADDRESS}"
  fi

  # Build the server cert renewal command (fully resolved, no variables)
  local _renew_server="/usr/bin/ziti pki create server"
  _renew_server+=" --pki-root '${_pki_root}'"
  _renew_server+=" --ca-name '${ZITI_INTERMEDIATE_FILE}'"
  _renew_server+=" --key-file '${ZITI_SERVER_FILE}'"
  _renew_server+=" --server-file '${ZITI_SERVER_FILE}'"
  _renew_server+=" --server-name '${_cert_name}'"
  _renew_server+=" --dns '${_dns_sans}'"
  _renew_server+=" --ip '${_ip_sans}'"
  _renew_server+=" --allow-overwrite"
  _renew_server+=" --spiffe-id 'controller/${ZITI_CLUSTER_NODE_NAME}'"

  # Build the client cert renewal command (fully resolved, no variables)
  local _renew_client="/usr/bin/ziti pki create client"
  _renew_client+=" --pki-root '${_pki_root}'"
  _renew_client+=" --ca-name '${ZITI_INTERMEDIATE_FILE}'"
  _renew_client+=" --key-file '${ZITI_SERVER_FILE}'"
  _renew_client+=" --client-file '${ZITI_CLIENT_FILE}'"
  _renew_client+=" --client-name '${_cert_name}'"
  _renew_client+=" --allow-overwrite"
  _renew_client+=" --spiffe-id 'controller/${ZITI_CLUSTER_NODE_NAME}'"

  # Paths for chown after renewal
  local _certs_dir="${_pki_root}/${ZITI_INTERMEDIATE_FILE}/certs"
  local _keys_dir="${_pki_root}/${ZITI_INTERMEDIATE_FILE}/keys"

  cat > "${_svc_unit}" <<UNIT
[Unit]
Description=Renew OpenZiti Controller leaf certificates
After=network-online.target

[Service]
Type=oneshot
User=${_svc_user}
Group=${_svc_group}
WorkingDirectory=${ZITI_HOME}

# Generated by bootstrap.bash on $(date --utc --iso-8601=seconds)
# Leaf certs valid 365 days; timer fires monthly.
ExecStart=/usr/bin/bash -c '${_renew_server}'
ExecStart=/usr/bin/bash -c '${_renew_client}'
UNIT

  cat > "${_timer_unit}" <<UNIT
[Unit]
Description=Monthly renewal of OpenZiti Controller leaf certificates

[Timer]
# Leaf certs are valid for 365 days by default.  Monthly renewal keeps certs
# fresh with ~11 months of buffer.  RandomizedDelaySec spreads renewal across
# the first week of each month to avoid thundering-herd in large deployments.
OnCalendar=monthly
RandomizedDelaySec=1w
Persistent=true

[Install]
WantedBy=timers.target
UNIT

  systemctl daemon-reload

  # Always issue the initial leaf certs so the controller can start.
  echo "INFO: issuing initial leaf certificates"
  systemctl start ziti-controller-cert-renewal.service

  # Enable the timer if auto-renewal is requested (interactive prompt or env var).
  # The units are always installed so an admin can enable the timer later.
  local _enable_timer="${ZITI_AUTO_RENEW_CERTS:-true}"
  if isInteractive && [[ "${_enable_timer}" == true ]]; then
    local _timer_answer
    _timer_answer="$(prompt "Enable automatic certificate renewal timer? [Y/n]: " || echo "y")"
    if [[ "${_timer_answer}" =~ ^([nN][oO]?)$ ]]; then
      _enable_timer=false
    fi
  fi
  if [[ "${_enable_timer}" == true ]]; then
    systemctl enable --now ziti-controller-cert-renewal.timer
    echo "INFO: cert renewal timer enabled (monthly, leaf certs valid 365 days)"
  else
    echo "INFO: cert renewal timer installed but not enabled"
    echo "  Enable later: systemctl enable --now ziti-controller-cert-renewal.timer"
  fi
}

dataDir() {
  if ! (( "${#}" )); then
    echo "ERROR: no config file path provided" >&2
    return 1
  fi
  local _config_file="${1}"
  awk -F: '/^[[:space:]]+dataDir:/ {print $2}' "${_config_file}" | xargs -r realpath
}

exitHandler() {
  echo >&2
  echo "Something went wrong." >&2
  if [[ -s "${INFO_LOG_FILE:-}" || -s "${DEBUG_LOG_FILE:-}" ]]; then
    cat "${INFO_LOG_FILE:-/dev/null}" "${DEBUG_LOG_FILE:-/dev/null}" >> "${BOOTSTRAP_LOG_FILE}"
    echo "  Output: ${BOOTSTRAP_LOG_FILE}" >&2
  fi
  if [[ -s "${BOOT_ENV_FILE:-}" ]]; then
    echo "  Answers: ${BOOT_ENV_FILE}" >&2
  fi
  echo >&2
  echo "Re-run:" >&2
  echo "  sudo /opt/openziti/etc/controller/bootstrap.bash${BOOT_ENV_FILE:+ ${BOOT_ENV_FILE}}" >&2
  echo >&2
  echo "Set DEBUG=1 for more output." >&2
}

# BEGIN

# set defaults
: "${ZITI_PKI_ROOT:=pki}"  # relative to working directory
: "${ZITI_CA_FILE:=root}"  # relative to ZITI_PKI_ROOT
: "${ZITI_INTERMEDIATE_FILE:=intermediate}"  # relative to ZITI_PKI_ROOT
: "${ZITI_SERVER_FILE:=server}"  # relative to intermediate CA "keys" and "certs" dirs
: "${ZITI_CLIENT_FILE:=client}"  # relative to intermediate CA "keys" and "certs" dirs
: "${ZITI_NETWORK_NAME:=ctrl}"  # basename of identity files
: "${ZITI_CTRL_BIND_ADDRESS:=0.0.0.0}"  # the interface address on which to listen
: "${BOOTSTRAP_LOG_FILE:=$(mktemp)}"  # where the exit handler should concatenate verbose and debug messages
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

  # Debug output and exit handler — only needed for direct execution.
  # When sourced (e.g., by entrypoint.bash), the caller manages its own
  # fd 3 and traps.
  : "${DEBUG:=0}"
  : "${VERBOSE:=1}"
  if (( DEBUG )); then
    exec 3>&1
    set -o xtrace
  else
    exec 3>>"${DEBUG_LOG_FILE:=$(mktemp)}"
  fi
  trap exitHandler EXIT

  export ZITI_HOME=/var/lib/ziti-controller
  SVC_ENV_FILE=/opt/openziti/etc/controller/service.env
  SVC_FILE=/etc/systemd/system/ziti-controller.service.d/override.conf
  : "${ZITI_CONSOLE_LOCATION:=/opt/openziti/share/console}"

  ANSWER_FILE=""
  if [[ "${1:-}" =~ ^[-] ]]
  then
    echo -e "\nUsage:"\
            "\n\t$0 [ANSWER_FILE]"\
            "\n" \
            "\nOPTIONS" \
            "\n" \
            "\nVERBOSE=1\tprint INFO" \
            "\nDEBUG=1\t\tprint DEBUG" \
            "\n" >&2
    hintLinuxBootstrap "${ZITI_HOME}"
    exit 1
  elif (( $# )); then
    ANSWER_FILE="$1"
  fi
  # config file is always the standard location
  set -- "${ZITI_HOME}/config.yml"
  echo "DEBUG: using config file: $*" >&3

  if [[ $UID != 0 ]]; then
    echo "ERROR: must be run as root" >&2
    exit 1
  fi

  prepareWorkingDir "${ZITI_HOME}"
  stashZitiEnv
  loadEnvFiles "${SVC_ENV_FILE}"  # feature flags (lowest precedence)
  restoreZitiEnv

  # Aggregate answers in a temp file — deleted on success.
  # Feature flags stay in service.env (the package conffile).
  # On failure the temp file survives for debugging.
  BOOT_ENV_FILE="$(mktemp)"
  if [[ -n "${ANSWER_FILE}" && -f "${ANSWER_FILE}" ]]; then
    echo "DEBUG: loading answers from ${ANSWER_FILE}" >&3
    loadEnvFiles "${ANSWER_FILE}"
  fi
  importZitiVars                # get ZITI_* vars from environment and set in BOOT_ENV_FILE
  promptBootstrap               # prompt for ZITI_BOOTSTRAP if explicitly disabled (set and != true)
  promptBootstrapCluster        # prompt for new cluster or existing PKI
  promptCtrlAddress             # prompt for ZITI_CTRL_ADVERTISED_ADDRESS if not already set
  promptClusterNodeName         # prompt for ZITI_CLUSTER_NODE_NAME (default derived from address)
  promptClusterTrustDomain      # prompt for ZITI_CLUSTER_TRUST_DOMAIN (default derived from address)
  promptClusterNodePki          # prompt for ZITI_CLUSTER_NODE_PKI if joining existing cluster
  promptCtrlPort                # prompt for ZITI_CTRL_ADVERTISED_PORT if not already set
  promptUser                    # prompt for ZITI_USER if not already set and database bootstrapping enabled
  promptPassword                # prompt for ZITI_PWD if not already set and database bootstrapping enabled
  promptConsole                 # prompt for ZAC console binding configuration

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

    # Install systemd timer + oneshot for leaf cert issuance and renewal.
    # Always installed when PKI is managed; the timer is only *enabled* if
    # ZITI_AUTO_RENEW_CERTS is true (checked inside the function).
    if [[ "${ZITI_BOOTSTRAP_PKI}" == true ]]; then
      installCertRenewalTimer
    fi

    # Initialize cluster with default admin if bootstrapping a new cluster
    if [[ "${ZITI_BOOTSTRAP_CLUSTER:-}" == true || "${ZITI_BOOTSTRAP_DATABASE:-}" == true ]]; then
      # Check if systemd is available (PID 1 is systemd) and auto-start is not skipped
      if [[ -d /run/systemd/system ]]; then
        echo "DEBUG: starting controller service for cluster initialization" >&3
        systemctl start ziti-controller.service

        # Get the controller PID and wait for agent
        _ctrl_pid="$(systemctl show -p MainPID --value ziti-controller.service)"
        if [[ -z "${_ctrl_pid}" || "${_ctrl_pid}" == 0 ]]; then
          echo "ERROR: unable to determine controller MainPID for cluster init" >&2
        else
          if waitForAgent "${_ctrl_pid}" 30 1; then
            clusterInit "${_ctrl_pid}"
          fi
        fi
      else
        echo "WARN: systemd not available, skipping cluster initialization (requires systemd when bootstrap.bash is executed directly)" >&2
      fi
    fi

    # if verbose then this was already done earlier, else allow stdout now to announce completion
    if ! (( VERBOSE )); then
      exec 1>&4
    fi
    # clean up temp answers file — config.yml is the source of truth now
    rm -f "${BOOT_ENV_FILE:-}"
    echo -e "INFO: bootstrap completed successfully and will not run again."\
            "Adjust ${ZITI_HOME}/config.yml to suit." >&2
    trap - EXIT  # remove exit trap

    # On Linux with systemd, enable and start the service if not already running
    if [[ -d /run/systemd/system ]]; then
      if ! systemctl is-enabled --quiet ziti-controller.service 2>/dev/null; then
        systemctl enable ziti-controller.service
      fi
      if ! systemctl is-active --quiet ziti-controller.service 2>/dev/null; then
        systemctl start ziti-controller.service
      fi
      echo "Run 'systemctl status ziti-controller' to verify." >&2
    fi
  else
    echo "ERROR: something went wrong during bootstrapping" >&2
  fi
fi
