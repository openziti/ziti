#!/usr/bin/env bash

#
# bootstrap the OpenZiti Controller with PKI, config file, and database
#

makePki() {
  #
  # create missing PKI
  #

  if [[ "${ZITI_BOOTSTRAP_PKI}" == true ]]; then
    issueRootCert
    issueSignerCert
    issueLeafCerts
  else
    echo "DEBUG: skipping PKI creation because ZITI_BOOTSTRAP_PKI=${ZITI_BOOTSTRAP_PKI}" >&3
  fi
}

issueRootCert(){
  # generate a root CA unless bootstrapping is disabled or it already exists
  if [[ "${ZITI_BOOTSTRAP_CLUSTER}" == true ]]; then
    if [[ -z "${ZITI_CLUSTER_TRUST_DOMAIN:-}" ]]; then
      echo "ERROR: ZITI_CLUSTER_TRUST_DOMAIN must be set to generate a new cluster PKI" >&2
      hintLinuxBootstrap "${PWD}"
      return 1
    fi

    ZITI_CA_CERT="${ZITI_PKI_ROOT}/${ZITI_CA_FILE}/certs/${ZITI_CA_FILE}.cert"
    ZITI_CA_KEY="${ZITI_PKI_ROOT}/${ZITI_CA_FILE}/keys/${ZITI_CA_FILE}.key"
    if [[ -s "${ZITI_CA_CERT}" && -s "${ZITI_CA_KEY}" ]]; then
      echo "DEBUG: cluster PKI already exists in $(realpath "${ZITI_CA_CERT}")" >&3
    elif [[ -s "${ZITI_CA_CERT}" || -s "${ZITI_CA_KEY}" ]]; then
      echo "ERROR: ${ZITI_CA_CERT} and ${ZITI_CA_KEY} must both exist or neither exist as non-empty files" >&2
      return 1
    else
      # generate private key and issue self-signed root CA cert for new cluster PKI
      echo "DEBUG: generating new cluster PKI" >&3
      ziti pki create ca \
        --pki-root "${ZITI_PKI_ROOT}" \
        --ca-file "${ZITI_CA_FILE}" \
        --trust-domain "$(normalizeTrustDomain ${ZITI_CLUSTER_TRUST_DOMAIN})"
    fi
  fi
}

issueSignerCert() {
  #
  # issue edge signer intermediate CA certificate unless disabled or already exists
  #

  if [[ "${ZITI_BOOTSTRAP_NODE:-}" == true ]]; then
    if [[ -n "${ZITI_CLUSTER_NODE_NAME:-}" ]]; then
      ZITI_INTERMEDIATE_FILE="${ZITI_CLUSTER_NODE_NAME}"
    else
      echo "ERROR: ZITI_CLUSTER_NODE_NAME must be set to issue a signer certificate" >&2
      return 1
    fi

    if [[ "${ZITI_CA_FILE}" == "${ZITI_INTERMEDIATE_FILE}" ]]; then
      echo "ERROR: ZITI_CA_FILE and ZITI_INTERMEDIATE_FILE must be different" >&2
      return 1
    fi

    ZITI_PKI_SIGNER_CERT="${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/certs/${ZITI_INTERMEDIATE_FILE}.cert"
    ZITI_PKI_SIGNER_KEY="${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/keys/${ZITI_INTERMEDIATE_FILE}.key"
    if [[ ! -s "${ZITI_PKI_SIGNER_CERT}" && ! -s "${ZITI_PKI_SIGNER_KEY}" ]]; then
      echo "DEBUG: generating new signer certificate" >&3
      ziti pki create intermediate \
        --pki-root "${ZITI_PKI_ROOT}" \
        --ca-name "${ZITI_CA_FILE}" \
        --intermediate-file "${ZITI_INTERMEDIATE_FILE}" \
        --intermediate-name "Ziti Edge Signer for $(normalizeTrustDomain ${ZITI_CLUSTER_TRUST_DOMAIN})/controller/${ZITI_CLUSTER_NODE_NAME}"
    elif [[ ! -s "${ZITI_PKI_SIGNER_CERT}" || ! -s "${ZITI_PKI_SIGNER_KEY}" ]]; then
      echo "ERROR: ${ZITI_PKI_SIGNER_CERT} and ${ZITI_PKI_SIGNER_KEY} must both exist or neither exist as non-empty files" >&2
      return 1
    fi

    if [[ -s "${ZITI_PKI_SIGNER_CERT}" && -s "${ZITI_PKI_SIGNER_KEY}" ]]; then
      echo "DEBUG: edge signer CA exists in $(realpath "${ZITI_PKI_SIGNER_CERT}")" >&3
    else
      # trunk-ignore(shellcheck/SC2312)
      echo "DEBUG: edge signer exists in $(realpath "${ZITI_PKI_SIGNER_CERT}")" >&3
    fi
  else
    echo "DEBUG: skipping signer certificate creation because ZITI_BOOTSTRAP_NODE=${ZITI_BOOTSTRAP_NODE:-}" >&3
  fi
}

# ensure string has prefix 'spiffe://'
normalizeTrustDomain(){
  if [[ -n "${1:-}" ]]; then
    local _trust_domain="$1"
    shift
  else
    echo "ERROR: no trust domain provided" >&2
    return 1
  fi
  echo "spiffe://${_trust_domain#spiffe://}"
}

issueLeafCerts() {
  #
  # create server and client keys
  #

  # used by "ziti pki create server" as DNS SAN
  if [[ -z "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" ]]; then
    echo "ERROR: ZITI_CTRL_ADVERTISED_ADDRESS must be set to issue leaf certificateS" >&2
    return 1
  fi

  if [[ -n "${ZITI_CLUSTER_NODE_NAME:-}" ]]; then
    # controller identity vars
    ZITI_INTERMEDIATE_FILE="${ZITI_CLUSTER_NODE_NAME}"
  else
    echo "ERROR: ZITI_CLUSTER_NODE_NAME must be set to issue leaf certificates" >&2
    return 1
  fi

  if [[ "${ZITI_SERVER_FILE}" == "${ZITI_CLIENT_FILE}" ]]; then
    echo "ERROR: ZITI_SERVER_FILE and ZITI_CLIENT_FILE must be different" >&2
    return 1
  fi

  if [[ "${ZITI_SERVER_FILE}" == "${ZITI_INTERMEDIATE_FILE}" ]]; then
    echo "ERROR: ZITI_SERVER_FILE and ZITI_INTERMEDIATE_FILE must be different" >&2
    return 1
  fi

  if [[ "${ZITI_CLIENT_FILE}" == "${ZITI_INTERMEDIATE_FILE}" ]]; then
    echo "ERROR: ZITI_CLIENT_FILE and ZITI_INTERMEDIATE_FILE must be different" >&2
    return 1
  fi

  if [[ -z "${ZITI_CLUSTER_NODE_NAME:-}" ]]; then
    echo "ERROR: ZITI_CLUSTER_NODE_NAME must be set" >&2
    hintLinuxBootstrap "${PWD}"
    return 1
  fi

  ZITI_PKI_CTRL_KEY="${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/keys/${ZITI_SERVER_FILE}.key"
  if ! [[ -s "${ZITI_PKI_CTRL_KEY}" ]]; then
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
  if [[ "${ZITI_AUTO_RENEW_CERTS}" == true || ! -s "${ZITI_PKI_CTRL_SERVER_CERT}" ]]; then

    local _dns_sans="localhost"
    local _ip_sans="127.0.0.1,::1"
    # trunk-ignore(shellcheck/SC2310)
    if isIpAddress "${ZITI_CTRL_ADVERTISED_ADDRESS:-}"; then
      _ip_sans+=",${ZITI_CTRL_ADVERTISED_ADDRESS}"
    else
      _dns_sans+=",${ZITI_CTRL_ADVERTISED_ADDRESS}"
    fi

    local _spiffe_id="controller/${ZITI_CLUSTER_NODE_NAME}"
    if [[ -n "${ZITI_CLUSTER_TRUST_DOMAIN:-}" ]]; then
      local _trust_domain=$(normalizeTrustDomain ${ZITI_CLUSTER_TRUST_DOMAIN})
      _spiffe_id="${_trust_domain}/${_spiffe_id}"
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
      --spiffe-id "${_spiffe_id}" \
      --allow-overwrite >&3  # write to debug fd because this runs every startup
  fi

  # client cert
  #   use the server key for both client and server certs until "ziti create config controller" supports separate keys for
  #   each
  ZITI_PKI_CTRL_CERT="${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/certs/${ZITI_CLIENT_FILE}.chain.pem"
  if [[ "${ZITI_AUTO_RENEW_CERTS}" == true || ! -s "${ZITI_PKI_CTRL_CERT}" ]]; then
    ziti pki create client \
      --pki-root "${ZITI_PKI_ROOT}" \
      --ca-name "${ZITI_INTERMEDIATE_FILE}" \
      --key-file "${ZITI_SERVER_FILE}" \
      --client-file "${ZITI_CLIENT_FILE}" \
      --client-name "${ZITI_CLUSTER_NODE_NAME}" \
      --spiffe-id "${_spiffe_id}" \
      --allow-overwrite >&3  # write to debug fd because this runs every startup
  fi

}

makeDatabase() {

  #
  # called by bootstrap() to initialize the database with default admin user and password
  #

  if [[ -n "${1:-}" ]]; then
    local _config_file="${1}"
    shift
  else
    echo "ERROR: no config file path provided" >&2
    return 1
  fi

  if [[ "${ZITI_BOOTSTRAP_CLUSTER:-}" == false ]]; then
    echo "DEBUG: skipping database initialization because ZITI_BOOTSTRAP_CLUSTER is false" >&3
    return 0
  elif [[ -d "${ZITI_CTRL_DATABASE_DIR}" && "${1:-}" == --force ]]; then
    echo "DEBUG: recreating database directory: ${ZITI_CTRL_DATABASE_DIR}" >&3
    mv --no-clobber "${ZITI_CTRL_DATABASE_DIR}"{,".${ZITI_BOOTSTRAP_NOW}.old"}
  elif [[ -d "${ZITI_CTRL_DATABASE_DIR}" ]]; then
    # the presence of the directory indicates that the database has already been initialized
    echo "DEBUG: database directory exists: ${ZITI_CTRL_DATABASE_DIR}" >&3
    return 0
  fi

  if [[ -z "${ZITI_USER:-}" || -z "${ZITI_PWD:-}" ]]; then
    echo  "ERROR: unable to initialize database because ZITI_USER and ZITI_PWD must both be set" >&2
    hintLinuxBootstrap "${PWD}"
    return 1
  fi

  # shellcheck disable=SC2174
  echo "DEBUG: creating database directory: ${ZITI_CTRL_DATABASE_DIR}" >&3
  mkdir -pm0700 "${ZITI_CTRL_DATABASE_DIR}"

  # Set up trap to kill the controller process when function exits
  local _init_pid=""
  _cleanup_init_pid() {
    if [[ -n "${_init_pid:-}" ]]; then
      echo "DEBUG: cleaning up controller process ${_init_pid}" >&3
      kill -9 "${_init_pid}"
    fi
  }

  timeout 20s nohup ziti controller run "${_config_file}" >&3 &
  _init_pid=$!
  trap _cleanup_init_pid EXIT

  local _attempts=10
  until ! (( --_attempts )) \
    || ziti agent cluster init "${ZITI_USER}" "${ZITI_PWD}" 'Default Admin' 2>&3; do
    sleep 1
  done
  _cleanup_init_pid

  # Remove trap before returning
  trap - EXIT
  
  if (( _attempts )); then
    echo "DEBUG: initialized Docker controller database with ${_attempts} remaining attempts" >&3
    return 0
  else
    echo "ERROR: failed to initialize database" >&2
    # do not leave behind a partially-initialized database directory because it prevents us from trying again
    rm -rf "${ZITI_CTRL_DATABASE_DIR}"
    echo "DEBUG: removed partially-initialized database directory: ${ZITI_CTRL_DATABASE_DIR}" >&3
    return 1
  fi
}

makeConfig() {
  #
  # called by bootstrap() to create the controller config file
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

  if [[ -n "${ZITI_CLUSTER_NODE_NAME:-}" ]]; then
    # controller identity vars
    ZITI_INTERMEDIATE_FILE="${ZITI_CLUSTER_NODE_NAME}"
  else
    echo "ERROR: ZITI_CLUSTER_NODE_NAME must be set to generate a configuration" >&2
    return 1
  fi

  export  ZITI_PKI_CTRL_SERVER_CERT="${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/certs/${ZITI_SERVER_FILE}.chain.pem" \
          ZITI_PKI_CTRL_CERT="${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/certs/${ZITI_CLIENT_FILE}.chain.pem" \
          ZITI_PKI_CTRL_KEY="${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/keys/${ZITI_SERVER_FILE}.key" \
          ZITI_PKI_CTRL_CA="${ZITI_PKI_ROOT}/${ZITI_CA_FILE}/certs/${ZITI_CA_FILE}.cert"

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

# return true if the argument is an IP address
isIpAddress() {
  # trunk-ignore(shellcheck/SC2310)
  if isIpV4 "${1}" || isIpV6 "${1}"; then
    return 0
  else
    return 1
  fi
}

# return true if the argument is an IPv4 address
isIpV4() {
  if [[ "${1}" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]; then
    return 0
  else
    return 1
  fi
}

# return true if the argument is an IPv6 address
isIpV6() {
  # trunk-ignore(shellcheck/SC2310)
  # trunk-ignore(shellcheck/SC2312)
  if [[ "$(unwrapIpLiteral "${1}")" =~ ^([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}$ ]]; then
    return 0
  else
    return 1
  fi
}

# return the argument with square brackets removed if it is an IP literal, e.g., an IPv6 address
unwrapIpLiteral() {
  if [[ "${1}" =~ ^\[(.*)\]$ ]]; then
    echo "${BASH_REMATCH[1]}"
  else
    echo "${1}"
  fi
}

# return true if the argument is a domain name, i.e., ndots >= 1
isDomainName() {
  # trunk-ignore(shellcheck/SC2310)
  if ! isIpAddress "${1}" && [[ "${1}" =~ ^[a-zA-Z0-9-]+(\.[a-zA-Z0-9-]+)+$ ]]; then
    return 0
  else
    return 1
  fi
}

# return true if interactive and response is not empty
isInteractive() {
  if [[ "${DEBIAN_FRONTEND:-}" != "noninteractive" && -t 0 ]]; then
    return 0
  else
    return 1
  fi
}

# return true if interactive and response is not empty
prompt() {
  # trunk-ignore(shellcheck/SC2310)
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

promptUserPwd() {
  # do nothing if database directory exists
  if [[ -d "${ZITI_CTRL_DATABASE_DIR}" ]]; then
    echo "DEBUG: not checking ZITI_USER or ZITI_PWD because '$(realpath "${ZITI_CTRL_DATABASE_DIR}")' exists" >&3
    return 0
  # or we're not bootstrapping a cluster (true, false)
  elif [[ "${ZITI_BOOTSTRAP_CLUSTER:-}" == false ]]; then
    echo "DEBUG: not checking ZITI_USER or ZITI_PWD because ZITI_BOOTSTRAP_CLUSTER=${ZITI_BOOTSTRAP_CLUSTER}" >&3
    return 0
  # or we're not bootstrapping a database (true, force, false)
  elif [[ "${ZITI_BOOTSTRAP_DATABASE:-}" == false ]]; then
    echo "DEBUG: not checking ZITI_USER or ZITI_PWD because ZITI_BOOTSTRAP_DATABASE=${ZITI_BOOTSTRAP_DATABASE}" >&3
    return 0
  fi
  promptUser
  promptPwd
}

promptPwd() {
  # do nothing if password is already defined in env
  if [[ -n "${ZITI_PWD:-}" ]]; then
    echo "DEBUG: ZITI_PWD is defined in ${BOOT_ENV_FILE}" >&3
    return 0
  fi
  GEN_PWD=$(head -c128 /dev/urandom | LC_ALL=C tr -dc 'A-Za-z0-9!@#$%^*_+~' | cut -c 1-12)
  # don't set a generated password if not interactive because it will be unknown
  if isInteractive && ZITI_PWD="$(prompt "Set password for '${ZITI_USER}' [${GEN_PWD}]: " || echo "${GEN_PWD}")"; then
    # temporarily set password in env file, then scrub after db init
    setAnswer "ZITI_PWD=${ZITI_PWD}" "${BOOT_ENV_FILE}"
  else
    echo "ERROR: ZITI_PWD is required" >&2
    return 1
  fi
}

promptUser() {
  # do nothing if user is already defined in env
  if [[ -n "${ZITI_USER:-}" ]]; then
    echo "DEBUG: ZITI_USER is defined in ${BOOT_ENV_FILE}" >&3
    return 0
  fi
  ZITI_USER="$(prompt "Enter the name of the default user [admin]: " || echo 'admin')" || true
  if [[ -n "${ZITI_USER:-}" ]]; then
    setAnswer "ZITI_USER=${ZITI_USER}" "${BOOT_ENV_FILE}"
  else
    echo "ERROR: missing ZITI_USER in ${BOOT_ENV_FILE}" >&2
    return 1
  fi
}

# if not a tty (stdin is redirected), then slurp answers from stdin, e.g., env
# assignments like ZITI_THING=abcd1234, one per line
loadEnvStdin() {
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
  if (( $#)); then
    local -a _env_files=("${@}")
  else
    local -a _env_files=("${BOOT_ENV_FILE}" "${SVC_ENV_FILE}")
  fi
  for _env_file in "${_env_files[@]}"; do
    if [[ -s "${_env_file}" ]]; then
      # shellcheck disable=SC1090
      source "${_env_file}"
    else
      echo "WARN: missing env file '${_env_file}'" >&2
    fi
  done
}

promptCtrlAddress() {
  if [[ -z "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" ]]; then
    ZITI_CTRL_ADVERTISED_ADDRESS="$(prompt "Enter DNS name of the controller [required]: ")"
    if [[ -z "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" ]]; then
      echo "ERROR: missing required DNS name ZITI_CTRL_ADVERTISED_ADDRESS in ${BOOT_ENV_FILE}" >&2
      return 1
    else
      setAnswer "ZITI_CTRL_ADVERTISED_ADDRESS=${ZITI_CTRL_ADVERTISED_ADDRESS}" "${BOOT_ENV_FILE}"
    fi
  fi
}

promptBootstrapCluster(){
  if [[ -z "${ZITI_BOOTSTRAP_CLUSTER:-}" ]]; then
    # trunk-ignore(shellcheck/SC2310)
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
    # trunk-ignore(shellcheck/SC2310)
    if isDomainName "${ZITI_CTRL_ADVERTISED_ADDRESS:-}"; then
      ZITI_CLUSTER_NODE_NAME="$(
        prompt "Enter the unique name for this node in the cluster [${ZITI_CTRL_ADVERTISED_ADDRESS%%.*}]: " \
        || echo "${ZITI_CTRL_ADVERTISED_ADDRESS%%.*}"
      )"
    else
      # trunk-ignore(shellcheck/SC2310)
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
    local _prompt="Enter the trust domain for the new cluster"
    # if the address is a domain name then suggest everything after the first dot for the trust domain
    # trunk-ignore(shellcheck/SC2310)
    if isDomainName "${ZITI_CTRL_ADVERTISED_ADDRESS:-}"; then
      ZITI_CLUSTER_TRUST_DOMAIN="$(
        # trunk-ignore(shellcheck/SC2310)
        prompt "${_prompt} [${ZITI_CTRL_ADVERTISED_ADDRESS#*.}]: " \
        || echo "${ZITI_CTRL_ADVERTISED_ADDRESS#*.}"
      )"
    else
      # trunk-ignore(shellcheck/SC2310)
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

# bootstrapping is "true" by default, so this function only takes action if no longer "true" i.e., it was explicitly
# disabled and bootstrap.bash was re-run, indicating a desire to re-enable bootstrapping
promptBootstrap() {
    # do not prompt if unset or set to true because executing interactively means we want bootstrapping
    if [[ -n "${ZITI_BOOTSTRAP:-}" && "${ZITI_BOOTSTRAP}" != true ]]; then
      ZITI_BOOTSTRAP="$(prompt 'Generate a default config [y/N]: ' || echo 'false')"
      if [[ "${ZITI_BOOTSTRAP}" =~ ^([yY]([eE][sS])?|[tT]([rR][uU][eE])?)$ ]]; then
          ZITI_BOOTSTRAP=true
      elif [[ "${ZITI_BOOTSTRAP}" =~ ^([nN][oO]?|[fF]([aA][lL][sS][eE])?)$ ]]; then
          ZITI_BOOTSTRAP=false
      else
          echo "ERROR: unexpected value: ZITI_BOOTSTRAP=${ZITI_BOOTSTRAP}" >&2
          return 1
      fi
      setAnswer "ZITI_BOOTSTRAP=${ZITI_BOOTSTRAP}" "${SVC_ENV_FILE}"
    fi
    if [[ "${ZITI_BOOTSTRAP:-}" == false ]]; then
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
      if grep -qE "^${_key}=['\"]?${_value}['\"]?[\s$]" "${_env_file}" 2>/dev/null; then
        return 0
      # set if unset
      elif grep -qE "^${_key}=" "${_env_file}" 2>/dev/null; then
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
  local dropin_dir="/etc/systemd/system/ziti-controller.service.d"
  local override_file="${dropin_dir}/override.conf"
  if ! systemctl cat ziti-controller.service | grep -qE '^AmbientCapabilities=CAP_NET_BIND_SERVICE'; then
    if [[ ! -s "${override_file}" ]]; then
      echo "INFO: Creating ${override_file} to grant CAP_NET_BIND_SERVICE."
      mkdir -p "${dropin_dir}"
      cat > "${override_file}" <<EOF
[Service]
AmbientCapabilities=CAP_NET_BIND_SERVICE
EOF
      systemctl daemon-reload
    else
      echo -e "WARNING: not patching existing ${override_file}. Run 'systemctl edit ziti-controller.service' to add the following lines:"\
      "\n\n  [Service]"\
      "\n  AmbientCapabilities=CAP_NET_BIND_SERVICE\n" >&2
    fi
  fi
}

# inherit vars and set answers
loadEnvVars() {
  for line in $(set | grep -e "^ZITI_" | sort); do
    # shellcheck disable=SC2013
    setAnswer "${line}" "${SVC_ENV_FILE}" "${BOOT_ENV_FILE}"
  done
}

# make ziti vars available in child processes
exportZitiVars() {
  for line in $(set | grep -e "^ZITI_" | sort); do
    # shellcheck disable=SC2013
    export "${line%=*}"
  done
}

# Docker passes the "run" command to entrypoint.bash, and this function is then called in-line to ensure that the PKI (via makePki(), which calls issueLeafCerts() to renew the controller's identity), config file, and database are created if they don't exist; on Linux, this function is only called by the running bootstrap.bash directly, and the system service passes "check" to entrypoint.bash, which only performs a preflight check for PKI, config file, and database before calling issueLeafCerts() directly
bootstrap() {
  if [[ -n "${1:-}" ]]; then
    local _ctrl_config_file="${1}"
    echo "DEBUG: using config: $(realpath "${_ctrl_config_file}")" >&3
  else
    echo "ERROR: no config file path provided" >&2
    return 1
  fi

  makePki

  # make config file unless explicitly disabled or it exists, set "force" to overwrite
  if [[ -s "${_ctrl_config_file}" && "${ZITI_BOOTSTRAP_CONFIG}"   != force ]]; then
    echo "INFO: config file exists in $(realpath "${_ctrl_config_file}")"
  elif [[ "${ZITI_BOOTSTRAP_CONFIG}" == true ]]; then
    makeConfig "${_ctrl_config_file}"
  elif [[ "${ZITI_BOOTSTRAP_CONFIG}" == force ]]; then
    makeConfig "${_ctrl_config_file}" --force
  elif [[ "${ZITI_BOOTSTRAP_CONFIG}" == false ]]; then
    echo "DEBUG: skipping config generation because ZITI_BOOTSTRAP_CONFIG=${ZITI_BOOTSTRAP_CONFIG}" >&3
  else
    echo "ERROR: unexpected value in ZITI_BOOTSTRAP_CONFIG=${ZITI_BOOTSTRAP_CONFIG}" >&2
    return 1
  fi

  # make database unless explicitly disabled or it exists
  if [[ "${ZITI_BOOTSTRAP_DATABASE}" == true ]]; then
    makeDatabase "${_ctrl_config_file}"
  elif [[ "${ZITI_BOOTSTRAP_DATABASE}" == force ]]; then
    makeDatabase "${_ctrl_config_file}" --force
  fi

}

prepareWorkingDir() {
  if [[ -n "${1:-}" ]]; then
    local _config_dir="$1"
    # trunk-ignore(shellcheck/SC2312)
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

# set filemodes and ownership for the service's working directory
finalizeWorkingDir() {
  if [[ -n "${1:-}" && "${1}" != "/var/lib/private/ziti-controller" ]]; then
    echo "DEBUG: not finalizing alternative working dir: $(realpath "${1}")" >&3
    return 0
  elif [[ -n "${1:-}" ]]; then
    local _config_dir="$1"
    # trunk-ignore(shellcheck/SC2312)
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
  if [[ "${ZITI_RUNTIME}" != 'systemd' ]]; then
    return 0
  fi
  local _work_dir="${1:-${PWD}}"

  echo -e "\nProvide a configuration in '${_work_dir}' or generate with:"\
          "\n* Set vars in'/opt/openziti/etc/controller/bootstrap.env'"\
          "\n* Run '/opt/openziti/etc/controller/bootstrap.bash'"\
          "\n* Run 'systemctl enable --now ziti-controller.service'"\
          "\n"
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

# defaults
: "${ZITI_CA_FILE:=root}"  # relative to ZITI_PKI_ROOT
: "${ZITI_SERVER_FILE:=server}"  # relative to intermediate CA "keys" and "certs" dirs
: "${ZITI_CLIENT_FILE:=client}"  # relative to intermediate CA "keys" and "certs" dirs
: "${ZITI_NETWORK_NAME:=ctrl}"  # basename of identity files
: "${ZITI_CTRL_BIND_ADDRESS:=0.0.0.0}"  # the interface address on which to listen
: "${ZITI_BOOTSTRAP_LOG_FILE:=$(mktemp)}"  # where the exit handler should concatenate verbose and debug messages

#constants
ZITI_PKI_ROOT="./pki"  # relative to working directory
ZITI_BOOTSTRAP_NOW="$(date --utc --iso-8601=seconds)"
ZITI_CTRL_DATABASE_DIR="./raft"

# if this file was sourced, then only define vars and functions and change working directory; else if exec'd then bootstrap()
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

  : "${ZITI_HOME:=/var/lib/private/ziti-controller}"
  export ZITI_HOME
  SVC_ENV_FILE=/opt/openziti/etc/controller/service.env
  BOOT_ENV_FILE=/opt/openziti/etc/controller/bootstrap.env
  : "${ZITI_CONSOLE_LOCATION:=/opt/openziti/share/console}"

  if [[ "${1:-}" =~ ^[-] ]]; then
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
  elif (( $# )); then
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
  loadEnvVars                   # get ZITI_* vars from environment and set in BOOT_ENV_FILE
  loadEnvStdin                  # slurp answers from stdin if it's not a tty
  loadEnvFiles                  # load lowest precedence vars from SVC_ENV_FILE then BOOT_ENV_FILE
  promptBootstrap               # prompt for ZITI_BOOTSTRAP if explicitly disabled (set and != true)
  promptCtrlAddress             # prompt for ZITI_CTRL_ADVERTISED_ADDRESS if not already set
  promptCtrlPort                # prompt for ZITI_CTRL_ADVERTISED_PORT if not already set
  promptBootstrapCluster        # prompt for new cluster or existing PKI
  promptClusterNodeName         # prompt for ZITI_CLUSTER_NODE_NAME if not already set
  promptClusterTrustDomain      # prompt for ZITI_CLUSTER_TRUST_DOMAIN if not already set
  promptUserPwd                 # prompt for ZITI_USER and ZITI_PWD if not already set
  loadEnvFiles                  # reload env files to source new answers from prompts

  # suppress normal output during bootstrapping unless VERBOSE
  exec 4>&1; exec 1>>"${INFO_LOG_FILE:=$(mktemp)}"
  if (( VERBOSE )); then
    exec 1>&4
  fi
  
  # run bootstrap()
  bootstrap "${@}"

  # set filemodes
  finalizeWorkingDir "${ZITI_HOME}"

  # successfully running this script directly means bootstrapping was enabled
  setAnswer "ZITI_BOOTSTRAP=true" "${SVC_ENV_FILE}"

  # if verbose then this was already done earlier, else allow stdout now to announce completion
  if ! (( VERBOSE )); then
    exec 1>&4
  fi
  echo -e "INFO: bootstrap completed successfully and will not run again."\
          "Adjust ${ZITI_HOME}/config.yml to suit." >&2

  # remove exit trap
  trap - EXIT

fi
