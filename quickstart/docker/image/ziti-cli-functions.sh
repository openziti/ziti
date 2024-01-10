#!/bin/bash

set -uo pipefail

# Global Variables
ASCI_WHITE='\033[01;37m'
ASCI_RESTORE='\033[0m'
ASCI_RED='\033[00;31m'
ASCI_GREEN='\033[00;32m'
ASCI_YELLOW='\033[00;33m'
ASCI_BLUE='\033[00;34m'
ASCI_PURPLE='\033[00;35m'
ZITIx_EXPRESS_COMPLETE=""

function WHITE {
  echo "${ASCI_WHITE}${1-}${ASCI_RESTORE}"
}
function RED {  # Generally used for ERROR
  echo "${ASCI_RED}${1-}${ASCI_RESTORE}"
}
function GREEN {  # Generally used for SUCCESS messages
  echo "${ASCI_GREEN}${1-}${ASCI_RESTORE}"
}
function YELLOW { # Generally used for WARNING messages
  echo "${ASCI_YELLOW}${1-}${ASCI_RESTORE}"
}
function BLUE {   # Generally used for directory paths
  echo "${ASCI_BLUE}${1-}${ASCI_RESTORE}"
}
function PURPLE { # Generally used for Express Install milestones.
  echo "${ASCI_PURPLE}${1-}${ASCI_RESTORE}"
}

function _wait_for_controller {
  local advertised_host_port="${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS}:${ZITI_CTRL_EDGE_ADVERTISED_PORT}"
  while [[ "$(curl -w "%{http_code}" -m 1 -s -k -o /dev/null https://"${advertised_host_port}"/edge/client/v1/version)" != "200" ]]; do
    echo "waiting for https://${advertised_host_port}"
    sleep 3
  done
}

function _wait_for_public_router {
  local advertised_host_port="${ZITI_ROUTER_NAME}:${ZITI_ROUTER_PORT}"
  local COUNTDOWN=10
  until [[ -s "${ZITI_HOME}/${ZITI_ROUTER_NAME}.cert" ]] \
    && openssl s_client \
      -connect "${advertised_host_port}" \
      -servername "${ZITI_ROUTER_NAME}" \
      -alpn "ziti-edge,h2,http/1.1" \
      -cert "${ZITI_HOME}/${ZITI_ROUTER_NAME}.cert" \
      -key "${ZITI_HOME}/${ZITI_ROUTER_NAME}.key" \
      <>/dev/null 2>&1 # client cert needed for a zero exit code
  do
    if (( COUNTDOWN-- )); then
      echo "INFO: waiting for https://${advertised_host_port}"
      sleep 3
    else
      echo "ERROR: timed out waiting for https://${advertised_host_port}" >&2
      return 1
    fi
  done
}

function _setup_ziti_home {
  _setup_ziti_network
  if [[ "${ZITI_HOME-}" == "" ]]; then export ZITI_HOME="${HOME}/.ziti/quickstart/${ZITI_NETWORK-}"; else echo "ZITI_HOME overridden: ${ZITI_HOME}"; fi
}

function _setup_ziti_env_path {
  _setup_ziti_network
  _setup_ziti_home
  if [[ "${ZITI_ENV_FILE-}" == "" ]]; then export ZITI_ENV_FILE="${ZITI_HOME}/${ZITI_NETWORK}.env"; else echo "ZITI_ENV_FILE overridden: ${ZITI_ENV_FILE}"; fi
}


function _setup_ziti_network {
  if [[ "ran" != "${_setup_ziti_network_run}" ]]; then
    if [[ "${ZITI_NETWORK-}" == "" ]]; then ZITI_NETWORK="$(hostname)"; export ZITI_NETWORK; else echo "ZITI_NETWORK overridden: ${ZITI_NETWORK}"; fi
    _setup_ziti_network_run="ran"
  fi
}

function _set_ziti_bin_dir {
  if [[ "${ZITI_BIN_DIR-}" == "" ]]; then export ZITI_BIN_DIR="${ZITI_HOME}/ziti-bin/ziti-${ZITI_BINARIES_VERSION}"; else echo "ZITI_BIN_DIR overridden: ${ZITI_BIN_DIR}"; fi
}

function _get_file_overwrite_permission {
  local file_path="${1-}"

  if [[ -f "${file_path}" ]]; then
    echo -en "This will overwrite the existing file, continue? (y/N) "
    read -r
    if [[ "${REPLY}" == [^Yy]* ]]; then
      echo -e "$(RED "  --- Cancelling overwrite ---")"
      return 1
    fi

    return 0
  fi
}

# removes duplicate strings in a list
function _dedupe_list {
  local list delimiter retVal
  list=${1-}
  if [[ "${list}" == "" ]]; then
    return 1
  fi
  delimiter=${2-}
  if [[ "${delimiter}" == "" ]]; then
    delimiter=","
  fi

  echo "${list}" | tr "'${delimiter}'" '\n' | sort -u | xargs | tr ' ' ','
}

# Checks if a value is likely an IP address
function _is_ip {
  local param pattern
  param="${1}"
  pattern="^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$"
  if [[ "${param}" =~ $pattern ]]; then
    return 0
  fi
  return 1
}

function _pki_client_server {
  local retVal dns_allow_list ZITI_CA_NAME_local ip_allow_list file_name
  _check_env_variable ZITI_PKI ZITI_BIN_DIR
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi
  dns_allow_list=${1-}
  ZITI_CA_NAME_local=$2
  ip_allow_list=$3
  file_name=$4

  if [[ "${ip_allow_list}" == "" ]]; then
    ip_allow_list="127.0.0.1"
  fi

  # Dedupe the lists
  dns_allow_list=$(_dedupe_list "${dns_allow_list}")
  ip_allow_list=$(_dedupe_list "${ip_allow_list}")

  if ! test -f "${ZITI_PKI}/${ZITI_CA_NAME_local}/keys/${file_name}-server.key"; then
    echo "Creating server cert from ca: ${ZITI_CA_NAME_local} for ${dns_allow_list} / ${ip_allow_list}"
    "${ZITI_BIN_DIR-}/ziti" pki create server --pki-root="${ZITI_PKI}" --ca-name "${ZITI_CA_NAME_local}" \
          --server-file "${file_name}-server" \
          --dns "${dns_allow_list}" --ip "${ip_allow_list}" \
          --server-name "${file_name} server certificate"
  else
    echo "Creating server cert from ca: ${ZITI_CA_NAME_local} for ${dns_allow_list}"
    echo "key exists"
  fi

  if ! test -f "${ZITI_PKI}/${ZITI_CA_NAME_local}/keys/${file_name}-client.key"; then
    echo "Creating client cert from ca: ${ZITI_CA_NAME_local} for ${dns_allow_list}"
    "${ZITI_BIN_DIR-}/ziti" pki create client --pki-root="${ZITI_PKI}" --ca-name "${ZITI_CA_NAME_local}" \
          --client-file "${file_name}-client" \
          --key-file "${file_name}-server" \
          --client-name "${file_name}"
  else
    echo "Creating client cert from ca: ${ZITI_CA_NAME_local} for ${dns_allow_list}"
    echo "key exists"
  fi
  echo " "
}

function _pki_create_ca {
  local cert retVal
  _check_env_variable ZITI_PKI ZITI_BIN_DIR
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi
  cert="${1}"

  echo "Creating CA: ${cert}"
  if ! test -f "${ZITI_PKI}/${cert}/keys/${cert}.key"; then
    "${ZITI_BIN_DIR}/ziti" pki create ca --pki-root="${ZITI_PKI}" --ca-file="${cert}" --ca-name="${cert} Root CA"
  else
    echo "key exists"
  fi
  echo " "
}

function _pki_create_intermediate {
  local retVal
  _check_env_variable ZITI_PKI ZITI_BIN_DIR
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi
  echo "Creating intermediate: ${1} ${2} ${3}"
  if ! test -f "${ZITI_PKI}/${2}/keys/${2}.key"; then
    "${ZITI_BIN_DIR}/ziti" pki create intermediate --pki-root "${ZITI_PKI}" --ca-name "${1}" \
          --intermediate-name "${2}" \
          --intermediate-file "${2}" --max-path-len "${3}"
  else
    echo "key exists"
  fi
  echo " "
}

# Checks that a specific command or set of commands exist on the path
function _check_prereq {
  local missing_requirements="" arg
  for arg
  do
    if ! [[ -x "$(command -v "${arg}")" ]]; then
      missing_requirements="${missing_requirements}\n* ${arg}"
    fi
  done
  # Are requirements missing if yes, stop here and help 'em out
  if ! [[ "" = "${missing_requirements}" ]]; then
      echo " "
      echo "You're missing one or more commands that are used in this script."
      echo "Please ensure the commands listed are on the path and then try again."
      echo -e "${missing_requirements}"
      echo " "
      echo " "
      return 1
  fi
}

# Disable shellcheck for parameter expansion error, this function supports multiple shells
# shellcheck disable=SC2296
# Check if an environment variable is set, if not, throw an error
function _check_env_variable() {
  local _error=false arg
  for arg
  do
    # Parameter expansion is different between shells
    if [[ -n "$ZSH_VERSION" ]]; then
      if [[ -z "${(P)arg}" ]]; then
        echo -e "  * ERROR: $(RED "${arg} is not set") "
        _error=true
      fi
    elif [[ -n "$BASH_VERSION" ]]; then
      if [[ -z "${!arg}" ]]; then
        echo -e "  * ERROR: $(RED "${arg} is not set") "
        _error=true
      fi
    else
      echo -e " * $(RED "Unsupported shell, supply a PR or log an issue on https://github.com/openziti/ziti") "
      return 1
    fi
  done

  if [[ "true" == "${_error}" ]]; then
    return 1
  else
    return 0
  fi
}

function _issue_preamble {
  echo -e "$(PURPLE "-------------------------------------------------------------")"
  echo -e "$(PURPLE "                          _   _     _")"
  echo -e "$(PURPLE "                    ____ (_) | |_  (_)")"
  echo -e "$(PURPLE "                   |_  / | | | __| | |")"
  echo -e "$(PURPLE "                    / /  | | | |_  | |")"
  echo -e "$(PURPLE "                   /___| |_|  \__| |_|")"
  echo ""
  echo -e "$(PURPLE "-------------------------------------------------------------")"
  echo ""
  echo "This script will make it trivial to set up a very simple environment locally which will allow you to start"
  echo "learning ziti. This environment is suitable for development work only and is not a decent representation of"
  echo "a fully redundant production-caliber network."
  echo ""
}

function _issue_greeting {
  echo "Please note that, by default, this script will write files to your home directory into a directory named .ziti."
  echo -n "The currently configured location for these files will be: "
  echo -e "$(BLUE "${ZITI_HOME}")"
  echo ""
  echo ""
  echo "  \----------------------------------\ "
  echo "   \                                  \        __ "
  echo "    \         Welcome To:              \       | \ "
  echo "     >        Ziti Express 2.0          >------|  \       ______ "
  echo "    /                                  /       --- \_____/**|_|_\____  | "
  echo "   /                                  /          \_______ --------- __>-} "
  echo "  /----------------------------------/              /  \_____|_____/   | "
  echo "                                                    *         | "
  echo "                                                             {O} "
  echo ""
  echo "Let's get started creating your local development network!"
  echo ""
}

# Clear all environment variables prefixed with ZITI_ (use -s parameter to do so without any output)
function unsetZitiEnv {
  local param1 zEnvVar envvar
  param1="${1-}"
  for zEnvVar in $(set | grep -e "^ZITI_" | sort); do
    envvar="$(echo "${zEnvVar}" | cut -d '=' -f1)"
    if [[ "-s" != "${param1-}" ]]; then echo "unsetting [${envvar}] ${zEnvVar}"; fi
    unset "${envvar}"
  done
  # Have to explicitly unset these (no ZITI_ prefix)
  unset ZITIx_EXPRESS_COMPLETE
  unset _setup_ziti_network_run
}

# Checks for explicit environment variables or set as defaults, also creating directories as needed
function setupEnvironment {
  local pwd_reply
  echo "Populating environment variables"
  # General Ziti Values
  _setup_ziti_network
  _setup_ziti_home

  # Get Controller Credentials
  if [[ "${ZITI_USER-}" == "" ]]; then export ZITI_USER="admin"; else echo "ZITI_USER overridden: ${ZITI_USER}"; fi
  if [[ "${ZITI_PWD-}" == "" ]]; then
    ZITI_PWD="$(LC_ALL=C tr -dc _A-Z-a-z-0-9 < /dev/urandom | head -c32)"
    echo -en "Do you want to keep the generated admin password '$ZITI_PWD'? (Y/n) "
    # shellcheck disable=SC2162
    read -r pwd_reply
    if [[ -z "${pwd_reply}" || ${pwd_reply} =~ [yY] ]]; then
      echo "INFO: using ZITI_PWD=${ZITI_PWD}"
    else
      echo -en "Type the preferred admin password and press <enter> "
      read -r ZITI_PWD
    fi
  else
    echo "ZITI_PWD overridden: ${ZITI_PWD}"
  fi

  # PKI Values
  if [[ "${ZITI_PKI-}" == "" ]]; then export ZITI_PKI="${ZITI_HOME}/pki"; else echo "ZITI_PKI overridden: ${ZITI_PKI}"; fi
  if [[ "${ZITI_PKI_SIGNER_CERT_NAME-}" == "" ]]; then export ZITI_PKI_SIGNER_CERT_NAME="${ZITI_NETWORK}-signing"; else echo "ZITI_PKI_SIGNER_CERT_NAME overridden: ${ZITI_PKI_SIGNER_CERT_NAME}"; fi
  if [[ "${ZITI_PKI_SIGNER_ROOTCA_NAME-}" == "" ]]; then export ZITI_PKI_SIGNER_ROOTCA_NAME="${ZITI_PKI_SIGNER_CERT_NAME}-root-ca"; else echo "ZITI_PKI_SIGNER_ROOTCA_NAME overridden: ${ZITI_PKI_SIGNER_ROOTCA_NAME}"; fi
  if [[ "${ZITI_PKI_SIGNER_INTERMEDIATE_NAME-}" == "" ]]; then export ZITI_PKI_SIGNER_INTERMEDIATE_NAME="${ZITI_PKI_SIGNER_CERT_NAME}-intermediate"; else echo "ZITI_PKI_SIGNER_INTERMEDIATE_NAME overridden: ${ZITI_PKI_SIGNER_INTERMEDIATE_NAME}"; fi
  if [[ "${ZITI_PKI_SIGNER_CERT}" == "" ]]; then export ZITI_PKI_SIGNER_CERT="${ZITI_PKI}/signing.pem"; else echo "ZITI_PKI_SIGNER_CERT overridden: ${ZITI_PKI_SIGNER_CERT}"; fi
  if [[ "${ZITI_PKI_SIGNER_KEY}" == "" ]]; then export ZITI_PKI_SIGNER_KEY="${ZITI_PKI}/${ZITI_PKI_SIGNER_INTERMEDIATE_NAME}/keys/${ZITI_PKI_SIGNER_INTERMEDIATE_NAME}.key"; else echo "ZITI_PKI_SIGNER_KEY overridden: ${ZITI_PKI_SIGNER_KEY}"; fi

  # Run these functions to populate other pertinent environment values
  _detect_architecture    # ZITI_ARCH
  _detect_OS              # ZITI_OSTYPE
  getLatestZitiVersion  # ZITI_BINARIES_FILE & ZITI_BINARIES_VERSION

  # Must run after the above (dependent on other variables)
  _set_ziti_bin_dir

  # Controller Values
  if [[ "${ZITI_CTRL_NAME-}" == "" ]]; then export ZITI_CTRL_NAME="${ZITI_NETWORK}"; else echo "ZITI_CTRL_NAME overridden: ${ZITI_CTRL_NAME}"; fi
  if [[ "${ZITI_CTRL_EDGE_NAME-}" == "" ]]; then export ZITI_CTRL_EDGE_NAME="${ZITI_NETWORK}-edge-controller"; else echo "ZITI_CTRL_EDGE_NAME overridden: ${ZITI_CTRL_EDGE_NAME}"; fi
  if [[ "${ZITI_CTRL_EDGE_ADVERTISED_PORT-}" == "" ]]; then export ZITI_CTRL_EDGE_ADVERTISED_PORT="1280"; else echo "ZITI_CTRL_EDGE_ADVERTISED_PORT overridden: ${ZITI_CTRL_EDGE_ADVERTISED_PORT}"; fi
  if [[ "${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS-}" == "" ]]; then export ZITI_CTRL_EDGE_ADVERTISED_ADDRESS="${ZITI_NETWORK-}"; else echo "ZITI_CTRL_EDGE_ADVERTISED_ADDRESS overridden: ${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS}"; fi
  if [[ "${ZITI_CTRL_BIND_ADDRESS-}" != "" ]]; then echo "ZITI_CTRL_BIND_ADDRESS overridden: ${ZITI_CTRL_BIND_ADDRESS}"; fi
  if [[ "${ZITI_CTRL_ADVERTISED_ADDRESS-}" == "" ]]; then export ZITI_CTRL_ADVERTISED_ADDRESS="${ZITI_NETWORK-}"; else echo "ZITI_CTRL_ADVERTISED_ADDRESS overridden: ${ZITI_CTRL_ADVERTISED_ADDRESS}"; fi
  if [[ "${ZITI_CTRL_ADVERTISED_PORT-}" == "" ]]; then export ZITI_CTRL_ADVERTISED_PORT="6262"; else echo "ZITI_CTRL_ADVERTISED_PORT overridden: ${ZITI_CTRL_ADVERTISED_PORT}"; fi
  if [[ "${ZITI_PKI_CTRL_ROOTCA_NAME-}" == "" ]]; then export ZITI_PKI_CTRL_ROOTCA_NAME="${ZITI_CTRL_ADVERTISED_ADDRESS}-root-ca"; else echo "ZITI_PKI_CTRL_ROOTCA_NAME overridden: ${ZITI_PKI_CTRL_ROOTCA_NAME}"; fi
  if [[ "${ZITI_PKI_CTRL_INTERMEDIATE_NAME-}" == "" ]]; then export ZITI_PKI_CTRL_INTERMEDIATE_NAME="${ZITI_CTRL_ADVERTISED_ADDRESS}-intermediate"; else echo "ZITI_PKI_CTRL_INTERMEDIATE_NAME overridden: ${ZITI_PKI_CTRL_INTERMEDIATE_NAME}"; fi
  if [[ "${ZITI_PKI_CTRL_EDGE_ROOTCA_NAME-}" == "" ]]; then export ZITI_PKI_CTRL_EDGE_ROOTCA_NAME="${ZITI_CTRL_EDGE_NAME}-root-ca"; else echo "ZITI_PKI_CTRL_EDGE_ROOTCA_NAME overridden: ${ZITI_PKI_CTRL_EDGE_ROOTCA_NAME}"; fi
  if [[ "${ZITI_PKI_CTRL_EDGE_INTERMEDIATE_NAME-}" == "" ]]; then export ZITI_PKI_CTRL_EDGE_INTERMEDIATE_NAME="${ZITI_CTRL_EDGE_NAME}-intermediate"; else echo "ZITI_PKI_CTRL_EDGE_INTERMEDIATE_NAME overridden: ${ZITI_PKI_CTRL_EDGE_INTERMEDIATE_NAME}"; fi
  if [[ "${ZITI_PKI_CTRL_SERVER_CERT-}" == "" ]]; then export ZITI_PKI_CTRL_SERVER_CERT="${ZITI_PKI}/${ZITI_PKI_CTRL_INTERMEDIATE_NAME}/certs/${ZITI_CTRL_ADVERTISED_ADDRESS}-server.chain.pem"; else echo "ZITI_PKI_CTRL_SERVER_CERT overridden: ${ZITI_PKI_CTRL_SERVER_CERT}"; fi
  if [[ "${ZITI_PKI_CTRL_KEY-}" == "" ]]; then export ZITI_PKI_CTRL_KEY="${ZITI_PKI}/${ZITI_PKI_CTRL_INTERMEDIATE_NAME}/keys/${ZITI_CTRL_ADVERTISED_ADDRESS}-server.key"; else echo "ZITI_PKI_CTRL_KEY overridden: ${ZITI_PKI_CTRL_KEY}"; fi
  if [[ "${ZITI_PKI_CTRL_CA-}" == "" ]]; then export ZITI_PKI_CTRL_CA="${ZITI_PKI}/cas.pem"; else echo "ZITI_PKI_CTRL_CA overridden: ${ZITI_PKI_CTRL_CA}"; fi
  if [[ "${ZITI_PKI_CTRL_CERT-}" == "" ]]; then export ZITI_PKI_CTRL_CERT="${ZITI_PKI}/${ZITI_PKI_CTRL_INTERMEDIATE_NAME}/certs/${ZITI_CTRL_ADVERTISED_ADDRESS}-client.cert"; else echo "ZITI_PKI_CTRL_CERT overridden: ${ZITI_PKI_CTRL_CERT}"; fi
  if [[ "${ZITI_PKI_EDGE_CERT-}" == "" ]]; then export ZITI_PKI_EDGE_CERT="${ZITI_PKI}/${ZITI_PKI_CTRL_EDGE_INTERMEDIATE_NAME}/certs/${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS}-client.cert"; else echo "ZITI_PKI_EDGE_CERT overridden: ${ZITI_PKI_EDGE_CERT}"; fi
  if [[ "${ZITI_PKI_EDGE_SERVER_CERT}" == "" ]]; then export ZITI_PKI_EDGE_SERVER_CERT="${ZITI_PKI}/${ZITI_PKI_CTRL_EDGE_INTERMEDIATE_NAME}/certs/${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS}-server.chain.pem"; else echo "ZITI_PKI_EDGE_SERVER_CERT overridden: ${ZITI_PKI_EDGE_SERVER_CERT}"; fi
  if [[ "${ZITI_PKI_EDGE_KEY}" == "" ]]; then export ZITI_PKI_EDGE_KEY="${ZITI_PKI}/${ZITI_PKI_CTRL_EDGE_INTERMEDIATE_NAME}/keys/${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS}-server.key"; else echo "ZITI_PKI_EDGE_KEY overridden: ${ZITI_PKI_EDGE_KEY}"; fi
  if [[ "${ZITI_PKI_EDGE_CA}" == "" ]]; then export ZITI_PKI_EDGE_CA="${ZITI_PKI}/${ZITI_PKI_CTRL_EDGE_ROOTCA_NAME}/certs/${ZITI_PKI_CTRL_EDGE_ROOTCA_NAME}.cert"; else echo "ZITI_PKI_EDGE_CA overridden: ${ZITI_PKI_EDGE_CA}"; fi

  # Router Values
  if [[ "${ZITI_ROUTER_NAME-}" == "" ]]; then export ZITI_ROUTER_NAME="${ZITI_NETWORK}-edge-router"; else echo "ZITI_ROUTER_NAME overridden: ${ZITI_ROUTER_NAME}"; fi
  if [[ "${ZITI_ROUTER_PORT-}" == "" ]]; then export ZITI_ROUTER_PORT="3022"; else echo "ZITI_ROUTER_PORT overridden: ${ZITI_ROUTER_PORT}"; fi
  if [[ "${ZITI_ROUTER_LISTENER_BIND_PORT-}" == "" ]]; then export ZITI_ROUTER_LISTENER_BIND_PORT="10080"; else echo "ZITI_ROUTER_LISTENER_BIND_PORT overridden: ${ZITI_ROUTER_LISTENER_BIND_PORT}"; fi
  if [[ "${EXTERNAL_DNS-}" != "" ]]; then export ZITI_ROUTER_ADVERTISED_ADDRESS="${EXTERNAL_DNS}"; fi

  # Set up directories
  mkdir -p "${ZITI_HOME}"
  mkdir -p "${ZITI_HOME}/db"
  mkdir -p "${ZITI_PKI}"

  _setup_ziti_env_path

  echo -e "$(GREEN "Your OpenZiti environment has been set up successfully.")"
  echo ""
}

# Stores environment variables prefixed with ZITI_ to a .env file
function persistEnvironmentValues {
  local filepath tmpfilepath retVal envval envvar zEnvVar
  # Get the file path
  filepath="${1-}"
  if [[ "" == "${filepath}" ]]; then
    _check_env_variable ZITI_ENV_FILE
    retVal=$?
    if [[ "${retVal}" != 0 ]]; then
      echo -e "$(RED "  --- persistEnvironment must take a parameter or have ZITI_ENV_FILE set ---")"
      return 1
    else
      filepath="${ZITI_ENV_FILE}"
    fi
  fi

  # Store all ZITI_ variables in the environment file, creating the directory if necessary
  tmpfilepath="$(mktemp)"
  mkdir -p "$(dirname "${filepath}")" && echo "" > "${tmpfilepath}"
  for zEnvVar in $(set | grep -e "^ZITI_" | sed "s/='\(.*\)'\$/=\1/" | sort); do
      envvar="$(echo "${zEnvVar}" | cut -d '=' -f1)"
      envval="$(echo "${zEnvVar}" | cut -d '=' -f2-1000)"
      echo 'if [[ "$'${envvar}'" == "" ]]; then export '${envvar}'="'${envval}'"; else echo "NOT OVERRIDING: env var '${envvar}' already set. using existing value"; fi' >> "${tmpfilepath}"
  done

  export PFXLOG_NO_JSON=true
  # shellcheck disable=SC2129
  echo "export PFXLOG_NO_JSON=true" >> "${tmpfilepath}"

  echo "alias zec='ziti edge'" >> "${tmpfilepath}"
  echo "alias zitiLogin='ziti edge login \"\${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS}:\${ZITI_CTRL_EDGE_ADVERTISED_PORT}\" -u \"\${ZITI_USER-}\" -p \"\${ZITI_PWD}\" -y'" >> "${tmpfilepath}"
  echo "alias psz='ps -ef | grep ziti'" >> "${tmpfilepath}"

  #when sourcing the emitted file add the bin folder to the path
  cat >> "${tmpfilepath}" <<'HEREDOC'
echo " "
if [[ ! "$(echo "$PATH"|grep -q "${ZITI_BIN_DIR}" && echo "yes")" == "yes" ]]; then
  echo "adding ${ZITI_BIN_DIR} to the path"
  export PATH=$PATH:"${ZITI_BIN_DIR}"
else
echo    "                  ziti binaries are located at: ${ZITI_BIN_DIR}"
echo -e 'add this to your path if you want by executing: export PATH=$PATH:'"${ZITI_BIN_DIR}"
echo " "
fi
HEREDOC

  mv "${tmpfilepath}" "${filepath}"
  echo -e "A file with all pertinent environment values was created here: $(BLUE "${filepath}")"
  echo ""
}

# Clears environment variables prefixed with ZITI_, and removes ziti environment directories
function removeZitiEnvironment {
  local specifiedVersion=""
  # No need to `_check_env_variable ZITI_VERSION_OVERRIDE ZITI_BINARIES_VERSION` as this will still run if they're blank
  echo -e "$(GREEN "Clearing existing Ziti variables and continuing with express install")"

  # Check if the user chose a specific version
  if [[ "${ZITI_VERSION_OVERRIDE-}" != "" ]] && [[ "${ZITI_VERSION_OVERRIDE-}" != "${ZITI_BINARIES_VERSION-}" ]]; then
    # Don't allow overriding the version if ziti quickstart was already run, the DB may not be compatible
    echo -e "$(RED "  --- Overriding the ziti version is not supported if the version differs from one already installed. ---")"
    echo -en "Would you like to continue by using the latest version? (y/N) "
    read -r
    echo " "
    if [[ "${REPLY}" == [Yy]* ]]; then
      unset ZITI_VERSION_OVERRIDE
    else
      return 1
    fi
  elif [[ "${ZITI_VERSION_OVERRIDE-}" != "" ]]; then
    echo -e "$(RED "  --- You have set the ZITI_VERSION_OVERRIDE value to ${ZITI_VERSION_OVERRIDE}. ---")"
    echo -en "Would you like to use this version again, choosing no will pull the latest version? (y/N) "
    read -r
    echo " "
    if [[ "${REPLY}" == [Yy]* ]]; then
      specifiedVersion="${ZITI_VERSION_OVERRIDE}"
    fi
  fi

  if [[ "${specifiedVersion}" != "" ]]; then
    export ZITI_VERSION_OVERRIDE="${specifiedVersion}"
  fi

  # Stop any devices currently running to avoid port collisions
  stopRouter
  stopController

  # Silently clear ziti variables (must be done after stopRouter and stopController)
  unsetZitiEnv "-s"
}

function startController {
  local retVal log_file pid
  _check_env_variable ZITI_HOME ZITI_BIN_DIR ZITI_CTRL_NAME
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi
  log_file="${ZITI_HOME-}/${ZITI_CTRL_NAME}.log"
  "${ZITI_BIN_DIR-}/ziti" controller run "${ZITI_HOME}/${ZITI_CTRL_NAME}.yaml" &> "${log_file}" 2>&1 &
  pid=$!
  echo -e "ziti controller started as process id: ${pid}. log located at: $(BLUE "${log_file}")"
}

# Disable unused args shellcheck, the arg is optional
#shellcheck disable=SC2120
function stopController {
  local pid retVal
  pid=${1-}
  if [[ "${pid}" == "" ]]; then
    _check_env_variable ZITI_CTRL_EDGE_ADVERTISED_PORT
    retVal=$?
    if [[ "${retVal}" != 0 ]]; then
      echo "You will need to source the ziti env file first or set ZITI_CTRL_EDGE_ADVERTISED_PORT so that the controller process can be found"
      return 1
    fi

    # Get the pid listening on the controller port
    pid=$(lsof -ti:"${ZITI_CTRL_EDGE_ADVERTISED_PORT}")
  fi

  if [[ -n ${pid:-} ]]; then
    kill "${pid}" > /dev/null 2>&1
    if [[ $? == 0 ]]; then
      echo "Controller stopped."
      return 0
    else
      echo "ERROR: Something went wrong while trying to stop the controller."
      return 1
    fi
  else
    echo "No process found."
  fi
}

function startRouter {
  local pid retVal log_file
  _check_env_variable ZITI_HOME ZITI_ROUTER_NAME ZITI_BIN_DIR
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi
  log_file="${ZITI_HOME}/${ZITI_ROUTER_NAME}.log"
  "${ZITI_BIN_DIR}/ziti" router run "${ZITI_HOME}/${ZITI_ROUTER_NAME}.yaml" > "${log_file}" 2>&1 &
  pid=$!
  echo -e "Express Edge Router started as process id: ${pid}. log located at: $(BLUE "${log_file}")"
}

# Disable unused args shellcheck, the arg is optional
#shellcheck disable=SC2120
function stopRouter {
  local pid retVal
  pid=${1-}
  if [[ "${pid}" == "" ]]; then
    _check_env_variable ZITI_ROUTER_PORT
    retVal=$?
    if [[ "${retVal}" != 0 ]]; then
      echo "You will need to source the ziti env file first so that the router process can be found"
      return 1
    fi

    # Get the pid listening on the controller port
    pid=$(lsof -ti:"${ZITI_ROUTER_PORT}")
  fi

  if [[ -n ${pid:-} ]]; then
    kill "${pid}" > /dev/null 2>&1
    if [[ $? == 0 ]]; then
      echo "Router stopped."
      return 0
    else
      echo "ERROR: Something went wrong while trying to stop the router." >&2
      return 1
    fi
  else
    echo "No process found."
  fi
}

# Checks all ports intended to be used in the Ziti network
function checkZitiPorts {
    local returnCnt=0
    _portCheck "ZITI_CTRL_ADVERTISED_PORT" "Controller"
    returnCnt=$((returnCnt + $?))
    _portCheck "ZITI_ROUTER_PORT" "Edge Router"
    returnCnt=$((returnCnt + $?))
    _portCheck "ZITI_CTRL_EDGE_ADVERTISED_PORT" "Edge Controller"
    returnCnt=$((returnCnt + $?))
    if [[ "${ZITI_ROUTER_LISTENER_BIND_PORT-}" != "" ]]; then
      # This port can be explicitly set but is not always, only check if set
      _portCheck "ZITI_ROUTER_LISTENER_BIND_PORT" "Router Listener Bind Port"
      returnCnt=$((returnCnt + $?))
    fi
    if [[ "returnCnt" -gt "0" ]]; then return 1; fi
    echo -e "$(GREEN "Expected ports are all available")"
    echo ""
}

# Detect which OS the script is running on and store it in a variable
function _detect_OS {
  if [ -n "${ZITI_OSTYPE}" ]; then return; fi
  if [[ "$OSTYPE" == "linux-gnu"* ]]; then
          export ZITI_OSTYPE="linux"
  elif [[ "$OSTYPE" == "darwin"* ]]; then
          export ZITI_OSTYPE="darwin"
  elif [[ "$OSTYPE" == "cygwin" ]]; then
          export ZITI_OSTYPE="windows"
  elif [[ "$OSTYPE" == "msys" ]]; then
          export ZITI_OSTYPE="windows"
  elif [[ "$OSTYPE" == "win32" ]]; then
          export ZITI_OSTYPE="windows"
  elif [[ "$OSTYPE" == "freebsd"* ]]; then
          echo -e "  * ERROR: $(RED "\$OSTYPE [$OSTYPE] is not supported at this time") "
          return 1
  else
          echo -e "  * ERROR: $(RED "\$OSTYPE is not set or is unknown: [$OSTYPE]. Cannot continue") "
          return 1
  fi
  return 0
}

# Detect which architecture the script is running on and store it in a variable
function _detect_architecture {
  local detected_arch
  if [ -n "${ZITI_ARCH}" ]; then return; fi
  _detect_OS
  ZITI_ARCH="amd64"
  detected_arch="$(uname -m)"
  # Apple M1 silicon
  if [[ "${detected_arch}" == *"arm"* ]] && [[ "${ZITI_OSTYPE}" == "darwin" ]]; then
    echo -e "$(YELLOW "WARN: It has been detected that you are using an Apple computer with ARM architecture. Deployment of Apple ARM architecture distributions is currently unsupported through git, the installer will pull darwin amd distribution instead.")"
  # LLVM 64 bit backends have merged so some versions of *nix use aarch64 while others use arm64 for parity with Apple
  elif [[ "${detected_arch}" == *"aarch64"* ]] || [[ "${detected_arch}" == *"arm64"* ]]; then
    ZITI_ARCH="arm64"
  elif [[ "${detected_arch}" == *"arm"* ]]; then
    ZITI_ARCH="arm"
  fi
}

function addZitiToPath {
  if [[ "${1-}" == "yes" ]]; then
    echo "Adding ${ZITI_BIN_DIR} to the path if necessary:"
    if [[ "$(echo "$PATH"|grep -q "${ZITI_BIN_DIR}" && echo "yes")" == "yes" ]]; then
      echo -e "$(GREEN "${ZITI_BIN_DIR}") is already on the path"
    else
      echo -e "adding $(RED "${ZITI_BIN_DIR}") to the path"
      export PATH=$PATH:"${ZITI_BIN_DIR}"
    fi
  fi
}

# Downloads and extracts ziti binaries onto the system. The latest version is used unless ZITI_VERSION_OVERRIDE is set.
function getZiti {
  local retVal default_path ziti_binaries_file_abspath zitidl reply
  _check_prereq curl jq tar 
  if [[ "${ZITI_BIN_DIR}" == "" ]]; then
    # Prompt user for input or use default
    _setup_ziti_home
    getLatestZitiVersion  # sets ZITI_BINARIES_FILE & ZITI_BINARIES_VERSION
    default_path="${ZITI_HOME}/ziti-bin/ziti-${ZITI_BINARIES_VERSION}"
    echo -en "The path for ziti binaries has not been set, use the default (${default_path})? (Y/n) "
    read -r reply
    if [[ -z "${reply}" || ${reply} =~ [yY] ]]; then
      echo "INFO: using the default path ${default_path}"
      ZITI_BIN_DIR="${default_path}"
    else
      echo -en "Enter the preferred fully qualified path and press <enter> (the path will be created if necessary) "
      read -r ZITI_BIN_DIR
    fi
  fi

  echo -e "Getting OpenZiti binaries"
  echo ""

  # Get the latest version unless a specific version is specified
  if [[ "${ZITI_VERSION_OVERRIDE-}" == "" ]]; then
    # If not overriding the version, determine the latest and populate ZITI_BINARIES_FILE ZITI_BINARIES_VERSION
    if ! getLatestZitiVersion; then
      return 1
    fi
  else
    _check_env_variable ZITI_BINARIES_FILE ZITI_BINARIES_VERSION
    retVal=$?
    if [[ "${retVal}" != 0 ]]; then
      return 1
    fi

    # Check if an error occurred while trying to pull desired version (happens with incorrect version or formatting issue)
    if ! _verify_ziti_version_exists; then
        echo -e "  * $(RED "ERROR: The version of ziti requested (${ZITI_VERSION_OVERRIDE}) could not be found for OS (${ZITI_OSTYPE}) and architecture (${ZITI_ARCH}). Please check these details and try again. The version should follow the format \"vx.x.x\".") "
        return 1
    fi
  fi

  # Where to store the ziti binaries zip
  ziti_binaries_file_abspath="${ZITI_BIN_DIR}/${ZITI_BINARIES_FILE}"
  # Check if they're already downloaded or maybe the user explicitly pointed ZITI_BIN_DIR to their local bins
  if ! test -f "${ZITI_BIN_DIR}/ziti"; then
    # Make the directory
    echo -e "No existing binary found, creating the ZITI_BIN_DIR directory ($(BLUE "${ZITI_BIN_DIR}"))"
    mkdir -p "${ZITI_BIN_DIR}"
    retVal=$?
    if [[ "${retVal}" != 0 ]]; then
      echo -e "  * $(RED "ERROR: An error occurred generating the path (${ZITI_BIN_DIR})")"
      return 1
    fi
  else
    echo -e "ziti found in ZITI_BIN_DIR ($(BLUE "${ZITI_BIN_DIR}"))"
    # Get the current version and compare with latest
    local currentVersion
    currentVersion="$("${ZITI_BIN_DIR}"/ziti --version)"
    if [[ "${ZITI_BINARIES_VERSION}" != "${currentVersion}" ]]; then
      # Prompt user for new download
      echo -en "There is a newer version of OpenZiti, would you like to download it? (Y/n) "
      read -r reply
      if [[ -z "${reply}" || "${reply}" == [Yy]* ]]; then
        # Update the ZITI_BIN_DIR path to point to the latest version
        unset ZITI_BIN_DIR
        _set_ziti_bin_dir
        # Make the directory
        mkdir -p "${ZITI_BIN_DIR}"
        retVal=$?
        if [[ "${retVal}" != 0 ]]; then
          echo -e "  * $(RED "ERROR: An error occurred generating the path (${ZITI_BIN_DIR}")"
          return 1
        fi

        # Update the .env file with the new downloaded version
        if ! test -f "${ZITI_ENV_FILE}"; then
          echo -e "  * $(YELLOW "WARN: The OpenZiti Environment file could not be found to update ziti binary related paths")"
        else
          sed -i.bak "s/export ZITI_BIN_DIR=.*/export ZITI_BIN_DIR=$(echo ${ZITI_BIN_DIR} | sed 's/\//\\\//g')/g" "${ZITI_ENV_FILE}"
          sed -i.bak "s/export ZITI_BINARIES_VERSION=.*/export ZITI_BINARIES_VERSION=$(echo ${ZITI_BINARIES_VERSION} | sed 's/\//\\\//g')/g" "${ZITI_ENV_FILE}"
          sed -i.bak "s/export ZITI_BINARIES_FILE=.*/export ZITI_BINARIES_FILE=$(echo ${ZITI_BINARIES_FILE} | sed 's/\//\\\//g')/g" "${ZITI_ENV_FILE}"
          sed -i.bak "/export ZITI_BINARIES_FILE_ABSPATH=.*/d" "${ZITI_ENV_FILE}"
        fi

        echo -e "$(YELLOW 'Getting latest binaries ')$(BLUE "${ZITI_BIN_DIR}")"
      else
        echo -e "$(YELLOW 'Using existing binaries at ')$(BLUE "${ZITI_BIN_DIR}")"
        addZitiToPath "$1"
        return 0
      fi
    else
      echo -e "$(YELLOW 'Latest binaries already exist, using existing binaries at ')$(BLUE "${ZITI_BIN_DIR}")"
      addZitiToPath "$1"
      return 0
    fi
  fi

  # Get the download link
  zitidl="https://github.com/openziti/ziti/releases/download/${ZITI_BINARIES_VERSION-}/${ZITI_BINARIES_FILE}"
  echo -e 'Downloading '"$(BLUE "${zitidl}")"' to '"$(BLUE "${ziti_binaries_file_abspath}")"
  curl -Ls "${zitidl}" -o "${ziti_binaries_file_abspath}"

  # Unzip the files
  tar -xf "${ziti_binaries_file_abspath}" --directory "${ZITI_BIN_DIR}"

  # Cleanup
  rm "${ziti_binaries_file_abspath}"      # Remove zip
  rm -rf "${ZITI_BIN_DIR}/ziti-extract"   # Remove extract folder

  # Mark the files executable
  chmod +x "${ZITI_BIN_DIR}/"*

  echo -e "$(GREEN "OpenZiti binaries ${ZITI_BINARIES_VERSION} successfully extracted to $(BLUE "${ZITI_BIN_DIR}")")"
  echo ""
  addZitiToPath "$1"
}

# Create a custom PKI
function createPki {
  local retVal pki_allow_list pki_allow_list_ip ZITI_GRANDPARENT_INTERMEDIATE
  _check_env_variable ZITI_PKI_CTRL_ROOTCA_NAME ZITI_PKI_CTRL_EDGE_ROOTCA_NAME ZITI_PKI_SIGNER_ROOTCA_NAME \
                      ZITI_PKI_SIGNER_INTERMEDIATE_NAME ZITI_PKI_CTRL_INTERMEDIATE_NAME \
                      ZITI_PKI_CTRL_EDGE_INTERMEDIATE_NAME
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi
  echo "Generating PKI"

  _pki_create_ca "${ZITI_PKI_CTRL_ROOTCA_NAME}"
  _pki_create_ca "${ZITI_PKI_CTRL_EDGE_ROOTCA_NAME}"
  _pki_create_ca "${ZITI_PKI_SIGNER_ROOTCA_NAME}"

  ZITI_GRANDPARENT_INTERMEDIATE="${ZITI_PKI_SIGNER_INTERMEDIATE_NAME}_grandparent_intermediate"
  _pki_create_intermediate "${ZITI_PKI_CTRL_ROOTCA_NAME}" "${ZITI_PKI_CTRL_INTERMEDIATE_NAME}" 1
  _pki_create_intermediate "${ZITI_PKI_CTRL_EDGE_ROOTCA_NAME}" "${ZITI_PKI_CTRL_EDGE_INTERMEDIATE_NAME}" 1
  _pki_create_intermediate "${ZITI_PKI_SIGNER_ROOTCA_NAME}" "${ZITI_GRANDPARENT_INTERMEDIATE}" 2
  _pki_create_intermediate "${ZITI_GRANDPARENT_INTERMEDIATE}" "${ZITI_PKI_SIGNER_INTERMEDIATE_NAME}" 1

  echo " "
  pki_allow_list="localhost,${ZITI_NETWORK}"
  if [[ "${ZITI_CTRL_ADVERTISED_ADDRESS-}" != "" ]]; then
    if ! _is_ip "${ZITI_CTRL_ADVERTISED_ADDRESS-}"; then
      pki_allow_list="${pki_allow_list},${ZITI_CTRL_ADVERTISED_ADDRESS}"
    else
      echo -e "$(YELLOW "ZITI_CTRL_ADVERTISED_ADDRESS seems to be an IP address, it will not be added to the SANs DNS list.") "
    fi
  fi
  if [[ "${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS-}" != "" ]]; then
    if ! _is_ip "${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS-}"; then
      pki_allow_list="${pki_allow_list},${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS}"
    else
      echo -e "$(YELLOW "ZITI_CTRL_EDGE_ADVERTISED_ADDRESS seems to be an IP address, it will not be added to the SANs DNS list.") "
    fi
  fi
  pki_allow_list_ip="127.0.0.1"
  if [[ "${ZITI_CTRL_EDGE_IP_OVERRIDE-}" != "" ]]; then
    pki_allow_list_ip="${pki_allow_list_ip},${ZITI_CTRL_EDGE_IP_OVERRIDE}"
  fi
  _pki_client_server "${pki_allow_list}" "${ZITI_PKI_CTRL_INTERMEDIATE_NAME}" "${pki_allow_list_ip}" "${ZITI_CTRL_ADVERTISED_ADDRESS}"

  pki_allow_list="localhost,${ZITI_NETWORK}"
  if [[ "${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS-}" != "" ]]; then
    if ! _is_ip "${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS-}"; then
      pki_allow_list="${pki_allow_list},${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS}"
    else
      echo -e "$(YELLOW "ZITI_CTRL_EDGE_ADVERTISED_ADDRESS seems to be an IP address, it will not be added to the SANs DNS list.") "
    fi
  fi
  pki_allow_list_ip="127.0.0.1"
  if [[ "${ZITI_CTRL_EDGE_IP_OVERRIDE-}" != "" ]]; then
    pki_allow_list_ip="${pki_allow_list_ip},${ZITI_CTRL_EDGE_IP_OVERRIDE}"
  fi
  _pki_client_server "${pki_allow_list}" "${ZITI_PKI_CTRL_EDGE_INTERMEDIATE_NAME}" "${pki_allow_list_ip}" "${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS}"

  echo -e "$(GREEN "PKI generated successfully")"
  echo -e ""
}

# Disable shellcheck un-passed arguments (arguments are optional)
# shellcheck disable=SC2120
# Creates a controller config file
function createControllerConfig {
  local controller_name retVal file_path output_file
  # Allow controller name to be passed in as arg
  controller_name="${1-}"
  # If no controller name provided and env var is not set, prompt user for a controller name
  if [[ "${controller_name}" == "" ]] && [[ -z "${ZITI_CTRL_NAME}" ]]; then
    echo -e "$(YELLOW "createControllerConfig requires a controller name to be supplied") "
    echo -en "Enter controller name: "
    read -r controller_name

    # Quit if no name is provided
    if [[ "${controller_name}" == "" ]]; then
      echo -e "$(RED "  --- Invalid controller name provided ---")"
      return 1
    fi
  # If no controller name provided and env var is set, use env var
  elif [[ "${controller_name}" == "" ]] && [[ -n "${ZITI_CTRL_NAME}" ]]; then
    controller_name="${ZITI_CTRL_NAME}"
  fi

  # Make sure necessary env variables are set
  # The following are used by ziti bin to generate the config so they need to be checked:
  #   ZITI_PKI_SIGNER_KEY ZITI_PKI_EDGE_CERT ZITI_PKI_EDGE_SERVER_CERT ZITI_PKI_EDGE_KEY ZITI_PKI_EDGE_CA
  _check_env_variable ZITI_PKI_CTRL_SERVER_CERT ZITI_PKI_CTRL_CA ZITI_PKI_SIGNER_CERT ZITI_PKI_SIGNER_KEY ZITI_BIN_DIR \
                      ZITI_PKI_EDGE_CERT ZITI_PKI_EDGE_SERVER_CERT ZITI_PKI_EDGE_KEY ZITI_PKI_EDGE_CA
  retVal=$?
  if [ $retVal -ne 0 ]; then
    return 1
  fi

  # Use the current directory if none is set
  file_path="${ZITI_HOME}"
  if [[ "${ZITI_HOME-}" == "" ]]; then file_path="."; fi

  echo "adding controller root CA to ca bundle: $ZITI_PKI/$ZITI_PKI_CTRL_ROOTCA_NAME/certs/$ZITI_PKI_CTRL_ROOTCA_NAME.cert"
  cat "$ZITI_PKI/$ZITI_PKI_CTRL_ROOTCA_NAME/certs/$ZITI_PKI_CTRL_ROOTCA_NAME.cert" > "${ZITI_PKI_CTRL_CA}"
  echo "adding signing root CA to ZITI_PKI_CTRL_CA: $ZITI_PKI_CTRL_CA"
  cat "$ZITI_PKI/$ZITI_PKI_SIGNER_ROOTCA_NAME/certs/$ZITI_PKI_SIGNER_ROOTCA_NAME.cert" >> "${ZITI_PKI_CTRL_CA}"
  echo -e "wrote CA file to: $(BLUE "${ZITI_PKI_CTRL_CA}")"
  
  echo "adding parent intermediate CA to ZITI_PKI_SIGNER_CERT: $ZITI_PKI_SIGNER_CERT"
  cat "$ZITI_PKI/$ZITI_PKI_SIGNER_INTERMEDIATE_NAME/certs/${ZITI_PKI_SIGNER_INTERMEDIATE_NAME}.cert" > "${ZITI_PKI_SIGNER_CERT}"
  echo "adding grandparent intermediate CA to ZITI_PKI_SIGNER_CERT: $ZITI_PKI_SIGNER_CERT"
  cat "$ZITI_PKI/$ZITI_PKI_SIGNER_ROOTCA_NAME/certs/${ZITI_PKI_SIGNER_INTERMEDIATE_NAME}_grandparent_intermediate.cert" >> "${ZITI_PKI_SIGNER_CERT}"
  echo -e "wrote signer cert file to: $(BLUE "${ZITI_PKI_SIGNER_CERT}")"

  output_file="${file_path}/${controller_name}.yaml"

  _get_file_overwrite_permission "${output_file}"
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  "${ZITI_BIN_DIR}/ziti" create config controller >"${output_file}"

  echo -e "Controller configuration file written to: $(BLUE "${output_file}")"
}

# Helper function to create a private edge router
function createPrivateRouterConfig {
  _create_router_config "${1-}" "private"
}

# Helper function to create a public edge router
function createEdgeRouterConfig {
  _create_router_config "${1-}" "public"
}

function createEdgeRouterWssConfig {
  _create_router_config "${1-}" "wss"
}

# Helper function to create a fabric router
function createFabricRouterConfig {
  _create_router_config "${1-}" "fabric"
}

# The main create router config function, all others point to this
function _create_router_config {
  local router_name router_type output_file retVal default_router_name file_path
  # Allow router name and type to be passed in as arg
  router_name="${1-}"
  router_type="${2-}"
  if [[ "${router_name}" == "" ]]; then

    # If router name is not passed as arg, prompt user for input
    echo -e "$(YELLOW "createEdgeRouterConfig requires a router name to be supplied") "
    default_router_name="${ZITI_ROUTER_NAME}"
    echo -en "Enter router name (${default_router_name}): "
    read -r router_name

    # Accept the default if no name provided
    if [[ "${router_name}" == "" ]]; then
      # Check for overwrite of default file
      router_name="${default_router_name}"
      _get_file_overwrite_permission "${ZITI_HOME-}/${router_name}.yaml"
      retVal=$?
      if [[ "${retVal}" != 0 ]]; then
        return 1
      fi
    fi
  fi
  # Get router type or set as default
  if [[ "${router_type}" == "" ]]; then
    router_type="private"
  elif [[ "private" != "${router_type}" ]] && [[ "public" != "${router_type}" ]] && [[ "fabric" != "${router_type}" ]] && [[ "wss" != "${router_type}" ]]; then
    echo -e "Unknown router type parameter provided '${router_type}', use 'public', 'private', 'fabric', or 'wss'"
  fi

  # Make sure necessary env variables are set
  # The following are used by ziti bin to generate the config so they need to be checked:
  # ZITI_CTRL_EDGE_ADVERTISED_ADDRESS ZITI_CTRL_ADVERTISED_PORT
  _check_env_variable ZITI_HOME ZITI_BIN_DIR ZITI_CTRL_EDGE_ADVERTISED_ADDRESS ZITI_CTRL_ADVERTISED_PORT
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  # Use the current directory if none is set
  file_path="${ZITI_HOME}"
  if [[ "${ZITI_HOME-}" == "" ]]; then file_path="."; fi

  output_file="${file_path}/${router_name}.yaml"

  _get_file_overwrite_permission "${output_file}"
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  if [[ "public" == "${router_type}" ]]; then
    "${ZITI_BIN_DIR}/ziti" create config router edge --routerName "${router_name}" > "${output_file}"
  elif [[ "private" == "${router_type}" ]]; then
    "${ZITI_BIN_DIR}/ziti" create config router edge --routerName "${router_name}" --private > "${output_file}"
  elif [[ "fabric" == "${router_type}" ]]; then
    "${ZITI_BIN_DIR}/ziti" create config router fabric --routerName "${router_name}" > "${output_file}"
  elif [[ "wss" == "${router_type}" ]]; then
    "${ZITI_BIN_DIR}/ziti" create config router edge --routerName "${router_name}" --wss > "${output_file}"
  fi
  echo -e "${router_type} router configuration file written to: $(BLUE "${output_file}")"
}

# Used to create a router, router config, then enroll the router.
function addRouter {
  local router_name router_type retVal router_attr
  # Make sure necessary env variables are set
  _check_env_variable ZITI_HOME ZITI_BIN_DIR ZITI_USER ZITI_PWD
  retVal=$?
  if [ $retVal -ne 0 ]; then
    return 1
  fi
  # Allow router name and type to be passed in as arg
  router_name="${1-}"
  router_type="${2-}"
  router_attr="${3-}"
  # If no router name provided and env var is not set, prompt user for a router name
  if [[ "${router_name}" == "" ]] && [[ -z "${ZITI_ROUTER_NAME}" ]]; then
    echo -e "$(YELLOW "addRouter requires a router name to be supplied") "
    echo -en "Enter router name: "
    read -r router_name

    # Quit if no name is provided
    if [[ "${router_name}" == "" ]]; then
      echo -e "$(RED "  --- Invalid router name provided ---")"
      return 1
    fi
  # If no router name provided and env var is set, use env var
  elif [[ "${router_name}" == "" ]] && [[ -n "${ZITI_ROUTER_NAME}" ]]; then
    router_name="${ZITI_ROUTER_NAME}"
  fi

  # Create router
  zitiLogin
  "${ZITI_BIN_DIR-}/ziti" edge delete edge-router "${router_name}"
  "${ZITI_BIN_DIR-}/ziti" edge create edge-router "${router_name}" -o "${ZITI_HOME}/${router_name}.jwt" -t -a "${router_attr}"

  # Create router config
  _create_router_config "${router_name}" "${router_type}"

  # Enroll the router
  "${ZITI_BIN_DIR-}/ziti" router enroll "${ZITI_HOME}/${router_name}.yaml" --jwt "${ZITI_HOME}/${router_name}.jwt" &> "${ZITI_HOME}/${router_name}.enrollment.log"
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    echo -e "$(RED "  --- There was an error during router enrollment, check the logs at ${ZITI_HOME}/${router_name}.enrollment.log ---")"
    return 1
  else
    echo -e "$(GREEN "Enrollment successful")"
  fi
}

function initializeController {
  local retVal log_file
  _setup_ziti_home
  # Make sure necessary env variables are set
  _check_env_variable ZITI_HOME ZITI_CTRL_NAME ZITI_USER ZITI_PWD ZITI_PKI_CTRL_CA ZITI_BIN_DIR
  retVal=$?
  if [ $retVal -ne 0 ]; then
    return 1
  fi

  log_file="${ZITI_HOME-}/${ZITI_CTRL_NAME}-init.log"
  "${ZITI_BIN_DIR-}/ziti" controller edge init "${ZITI_HOME}/${ZITI_CTRL_NAME}.yaml" -u "${ZITI_USER-}" -p "${ZITI_PWD}" &> "${log_file}"
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    echo -e "$(RED "  --- There was an error while initializing the controller, check the logs at ${log_file} ---")"
    return 1
  fi
  echo -e "${ZITI_CTRL_NAME} initialized. See $(BLUE "${log_file}") for details"
}

function zitiLogin {
  local advertised_host_port="${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS}:${ZITI_CTRL_EDGE_ADVERTISED_PORT}"
  "${ZITI_BIN_DIR-}/ziti" edge login "${advertised_host_port}" -u "${ZITI_USER-}" -p "${ZITI_PWD}" -y 2>&1
}

function expressInstall {
  local retVal
  # Check if expressInstall has been run before
  if [[ "" != "${ZITIx_EXPRESS_COMPLETE-}" ]]; then
    echo -e "$(RED "  --- It looks like you've run an express install in this shell already. ---")"
    echo -en "Would you like to clear existing Ziti variables and continue? (y/N) "
    read -r
    echo " "
    if [[ "${REPLY}" == [Yy]* ]]; then
      removeZitiEnvironment
    else
      echo -e "$(RED "  --- Exiting express install ---")"
      return 1
    fi
  fi
  export ZITIx_EXPRESS_COMPLETE="true"
  _issue_preamble

  # This is redundant but better to check here to prevent going any further
  _check_prereq curl jq tar hostname lsof
  retVal=$?
  if [ $retVal -ne 0 ]; then
    return 1
  fi
  _issue_greeting

  echo -e "$(PURPLE "******** Setting Up Your OpenZiti Environment ********")"
  # If a parameter was provided, set the network name to this value
  if [[ "${1-}" != "" ]]; then
    ZITI_NETWORK="${1-}"
  fi
  setupEnvironment
  persistEnvironmentValues ""

  echo -e "$(PURPLE "********      Getting OpenZiti Binaries       ********")"
  if ! getZiti "no"; then
    echo -e "$(RED "getZiti failed")"
    return 1
  fi

  # Check Ports
  echo -e "$(PURPLE "******** Ensure the Necessary Ports Are Open  ********")"
  if ! checkZitiPorts; then
    echo "Please clear the unavailable ports or change their values and try again."
    return 1
  fi

  # Create PKI
  echo -e "$(PURPLE "******** Generating Public Key Infrastructure ********")"
  createPki

  echo -e "$(PURPLE "********         Setting Up Controller        ********")"
  createControllerConfig
  if ! initializeController; then
    return 1
  fi
  startController
  echo "waiting for the controller to come online to allow the edge router to enroll"
  _wait_for_controller
  echo ""

  echo -e "$(PURPLE "******** Setting Up Edge Router ********")"
  zitiLogin
  echo ""
  echo -e "----------  Creating an edge router policy allowing all identities to connect to routers with a $(GREEN "#public") attribute"
  "${ZITI_BIN_DIR-}/ziti" edge delete edge-router-policy allEdgeRouters > /dev/null
  "${ZITI_BIN_DIR-}/ziti" edge create edge-router-policy allEdgeRouters --edge-router-roles '#public' --identity-roles '#all' > /dev/null

  echo -e "----------  Creating a service edge router policy allowing all services to use $(GREEN "#public") edge routers"
  "${ZITI_BIN_DIR-}/ziti" edge delete service-edge-router-policy allSvcAllRouters > /dev/null
  "${ZITI_BIN_DIR-}/ziti" edge create service-edge-router-policy allSvcAllRouters --edge-router-roles '#all' --service-roles '#all' > /dev/null
  echo ""

  echo "USING ZITI_ROUTER_NAME: $ZITI_ROUTER_NAME"

  addRouter "${ZITI_ROUTER_NAME}" "public" "public"
  echo ""

  stopController
  echo "Edge Router enrolled."

  echo ""
  echo -e "$(GREEN "Congratulations. Express setup complete!")"
  echo -e "Your ZITI_HOME is located here: $(BLUE "${ZITI_HOME}")"
  echo -e "Your admin password is: $(BLUE "${ZITI_PWD}")"
  echo ""
  echo -e "Start your Ziti Controller by running the function: $(BLUE "startController")"
  echo -e "Start your Ziti Edge Router by running : $(BLUE 'startRouter')"
  echo ""
}

# Gets the latest Ziti binary (the process is different for latest vs older so unfortunately two functions are needed)
function getLatestZitiVersion {
  local ziti_latest
  if ! _detect_OS; then
    return 1
  fi

  _detect_architecture

  ziti_latest=$(curl -s https://${GITHUB_TOKEN:+${GITHUB_TOKEN}@}api.github.com/repos/openziti/ziti/releases/latest)
  ZITI_BINARIES_FILE=$(printf "%s" "${ziti_latest}" | tr '\r\n' ' ' | jq -r '.assets[] | select(.name | startswith("'"ziti-${ZITI_OSTYPE}-${ZITI_ARCH}-"'")) | .name')
  ZITI_BINARIES_VERSION=$(printf "%s" "${ziti_latest}" | tr '\r\n' ' ' | jq -r '.tag_name')
}

function createControllerSystemdFile {
  local controller_name retVal output_file
  # Allow controller name to be passed in as an arg
  controller_name="${1-}"
  # If no controller name provided and env var is not set, prompt user for a controller name
  if [[ "${controller_name}" == "" ]]; then
    controller_name="${ZITI_NETWORK}"
  fi

  # Make sure necessary env variables are set
  _check_env_variable ZITI_HOME ZITI_BIN_DIR
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  output_file="${ZITI_HOME}/${controller_name}.service"

  _get_file_overwrite_permission "${output_file}"
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

cat > "${output_file}" <<HeredocForSystemd
[Unit]
Description=Ziti-Controller
After=network.target

[Service]
User=root
WorkingDirectory=${ZITI_HOME}
ExecStart="${ZITI_BIN_DIR}/ziti" controller run "${ZITI_HOME}/${controller_name}.yaml"
Restart=always
RestartSec=2
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target

HeredocForSystemd
  echo -e "Controller systemd file written to: $(BLUE "${output_file}")"
}

function createRouterSystemdFile {
  local router_name default_router_name retVal output_file
  # Allow router name to be passed in as an arg
  router_name="${1-}"
  if [[ "${router_name}" == "" ]]; then

    # If router name is not passed as arg, prompt user for input
    echo -e "$(YELLOW "createRouterSystemdFile requires a router name to be supplied") "
    default_router_name="${ZITI_ROUTER_NAME}"
    echo -en "Enter router name (${default_router_name}): "
    read -r router_name

    # Accept the default if no name provided
    if [[ "${router_name}" == "" ]]; then
      # Check for overwrite of default file
      router_name="${default_router_name}"
      _get_file_overwrite_permission "${ZITI_HOME-}/${router_name}.service"
      retVal=$?
      if [[ "${retVal}" != 0 ]]; then
        return 1
      fi
    fi
  fi

  _check_env_variable ZITI_HOME ZITI_BIN_DIR
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  output_file="${ZITI_HOME}/${router_name}.service"

  _get_file_overwrite_permission "${output_file}"
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

cat > "${output_file}" <<HeredocForSystemd
[Unit]
Description=Ziti-Router for ${router_name}
After=network.target

[Service]
User=root
WorkingDirectory=${ZITI_HOME}
ExecStart="${ZITI_BIN_DIR}/ziti" router run "${ZITI_HOME}/${router_name}.yaml"
Restart=always
RestartSec=2
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target

HeredocForSystemd
  echo -e "Router systemd file written to: $(BLUE "${output_file}")"
}

function createBrowZerSystemdFile {
  local retVal output_file node_bin
  _check_env_variable ZITI_HOME
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  output_file="${ZITI_HOME}/browzer-bootstrapper.service"

  if which node >/dev/null; then
    # store the absolute path to the node executable because it's required by systemd on Amazon Linux, at least
    node_bin=$(readlink -f "$(which node)")
  else
    echo "ERROR: missing executable 'node'" >&2
    return 1
  fi

  _get_file_overwrite_permission "${output_file}"
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  cat > "${output_file}" << HeredocForSystemd
[Unit]
Description=A systemd unit file for the Ziti BrowZer Bootstrapper
After=network.target

[Service]
User=root
EnvironmentFile=${ZITI_HOME}/browzer.env
WorkingDirectory=${ZITI_HOME}/ziti-browzer-bootstrapper
ExecStart=${node_bin} "${ZITI_HOME}/ziti-browzer-bootstrapper/index.js"
Restart=always
RestartSec=2
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target

HeredocForSystemd
  echo -e "Ziti BrowZer Bootstrapper systemd file written to: $(BLUE "${output_file}")"
}

function createControllerLaunchdFile {
  local controller_name retVal output_file
  # Allow controller name to be passed in as arg
  controller_name="${1-}"
  # If no controller name provided and env var is not set, prompt user for a controller name
  if [[ "${controller_name}" == "" ]] && [[ -z "${ZITI_CTRL_NAME}" ]]; then
        echo -e "$(YELLOW "createControllerLaunchdFile requires a controller name to be supplied") "
        echo -en "Enter controller name: "
        read -r controller_name

        # Quit if no name is provided
        if [[ "${controller_name}" == "" ]]; then
          echo -e "$(RED "  --- Invalid controller name provided ---")"
          return 1
        fi
  # If no controller name provided and env var is set, use env var
  elif [[ "${controller_name}" == "" ]] && [[ -n "${ZITI_CTRL_NAME}" ]]; then
    controller_name="${ZITI_CTRL_NAME}"
  fi

  # Make sure necessary env variables are set
  _check_env_variable ZITI_HOME ZITI_BIN_DIR
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  output_file="${ZITI_HOME}/${controller_name}.plist"

  _get_file_overwrite_permission "${output_file}"
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

cat > "${output_file}" <<HeredocForLaunchd
<?xml version="1.0" encoding="UTF-8"?>
  <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "https://www.apple.com/DTDs/PropertyList-1.0.dtd">
  <plist version="1.0">
    <dict>
      <key>Label</key>
      <string>ziti-controller-${controller_name}</string>
      <key>ProgramArguments</key>
      <array>
        <string>$ZITI_BIN_DIR/ziti</string>
        <string>controller</string>
        <string>run</string>
        <string>$ZITI_HOME/${controller_name}.yaml</string>
      </array>
      <key>WorkingDirectory</key>
      <string>${ZITI_HOME}</string>
      <key>KeepAlive</key>
      <dict>
        <key>PathState</key>
        <dict>
          <key>${ZITI_HOME}/launchd-enabled</key>
          <true/>
        </dict>
      </dict>
      <key>StandardOutPath</key>
      <string>${ZITI_HOME}/Logs/${controller_name}-{ZITI_BINARIES_VERSION}.log</string>
      <key>StandardErrorPath</key>
      <string>${ZITI_HOME}/Logs/${controller_name}-{ZITI_BINARIES_VERSION}.log</string>
    </dict>
  </plist>
HeredocForLaunchd
  echo -e "Controller launchd file written to: $(BLUE "${output_file}")"

  showLaunchdMessage
}

function createRouterLaunchdFile {
  local router_name default_router_name retVal output_file
  # Allow router name to be passed in as arg
  router_name="${1-}"
  if [[ "${router_name}" == "" ]]; then

    # If router name is not passed as arg, prompt user for input
    echo -e "$(YELLOW "createRouterLaunchdFile requires a router name to be supplied") "
    default_router_name="${ZITI_ROUTER_NAME}"
    echo -en "Enter router name (${default_router_name}): "
    read -r router_name

    # Accept the default if no name provided
    if [[ "${router_name}" == "" ]]; then
      # Check for overwrite of default file
      router_name="${default_router_name}"
      _get_file_overwrite_permission "${ZITI_HOME-}/${router_name}.plist"
      retVal=$?
      if [[ "${retVal}" != 0 ]]; then
        return 1
      fi
    fi
  fi

  # Make sure necessary env variables are set
  _check_env_variable ZITI_HOME ZITI_BIN_DIR
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  output_file="${ZITI_HOME-}/${router_name}.plist"

  _get_file_overwrite_permission "${output_file}"
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

cat > "${output_file}" <<HeredocForLaunchd
<?xml version="1.0" encoding="UTF-8"?>
  <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "https://www.apple.com/DTDs/PropertyList-1.0.dtd">
  <plist version="1.0">
    <dict>
      <key>Label</key>
      <string>$router_name</string>
      <key>ProgramArguments</key>
      <array>
        <string>$ZITI_BIN_DIR/ziti</string>
        <string>router</string>
        <string>run</string>
        <string>$ZITI_HOME/ctrl.with.edge.yml</string>
      </array>
      <key>WorkingDirectory</key>
      <string>${ZITI_HOME}</string>
      <key>KeepAlive</key>
      <true/>
      <dict>
        <key>PathState</key>
        <dict>
          <key>${ZITI_HOME}/launchd-enabled</key>
          <true/>
        </dict>
      </dict>
      <key>StandardOutPath</key>
      <string>${ZITI_HOME}/Logs/${router_name}-${ZITI_BINARIES_VERSION}.log</string>
      <key>StandardErrorPath</key>
      <string>${ZITI_HOME}/Logs/${router_name}-${ZITI_BINARIES_VERSION}.log</string>
    </dict>
  </plist>
HeredocForLaunchd
  echo -e "Router launchd file written to: $(BLUE "${output_file}")"

  showLaunchdMessage
}

function showLaunchdMessage {
  echo -e " "
  echo -e "$(YELLOW "The generated launchd file is designed to keep the service alive while the file")"
  echo -e "$(BLUE "${ZITI_HOME}/launchd-enabled") $(YELLOW "remains present.")"
  echo -e "$(YELLOW "If this file is not present, the service will end.")"
}

function createZacSystemdFile {
  local retVal output_file node_bin
  _check_env_variable ZITI_HOME
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  output_file="${ZITI_HOME}/ziti-console.service"

  _get_file_overwrite_permission "${output_file}"
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  if which node >/dev/null; then
    # store the absolute path to the node executable because it's required by systemd on Amazon Linux, at least
    node_bin=$(readlink -f "$(which node)")
  else
    echo "ERROR: missing executable 'node'" >&2
    return 1
  fi

cat > "${output_file}" <<HeredocForSystemd
[Unit]
Description=Ziti-Console
After=network.target

[Service]
User=root
WorkingDirectory=${ZITI_HOME}/ziti-console
ExecStart=${node_bin} "${ZITI_HOME}/ziti-console/server.js"
Restart=always
RestartSec=2
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target

HeredocForSystemd
  echo -e "ziti-console systemd file written to: $(BLUE "${output_file}")"
}

# Ensure that the version desired as specified by ZITI_VERSION_OVERRIDE exists, this returns an error in cases where
# the version doesn't exist or possibly just the version format provided in ZITI_VERSION_OVERRIDE is incorrect.
function _verify_ziti_version_exists {
  local ziticurl

  _detect_architecture

  ziticurl="$(curl -s https://${GITHUB_TOKEN:+${GITHUB_TOKEN}@}api.github.com/repos/openziti/ziti/releases/tags/"${ZITI_VERSION_OVERRIDE}")"
  ZITI_BINARIES_FILE=$(echo "${ziticurl}" | tr '\r\n' ' ' | jq -r '.assets[] | select(.name | startswith("'"ziti-${ZITI_OSTYPE}-${ZITI_ARCH}-"'")) | .name')
  ZITI_BINARIES_VERSION=$(echo "${ziticurl}" | tr '\r\n' ' ' | jq -r '.tag_name')

  # Check if there was an error while trying to get the requested version
  if [[ "${ZITI_BINARIES_VERSION-}" == "null" ]]; then
    echo "ERROR: response missing '.tag_name': ${ziticurl}" >&2
    return 1
  fi

  echo "The ziti version requested (${ZITI_BINARIES_VERSION}) was verified and has been stored in ZITI_BINARIES_VERSION"
}

# Disable shellcheck for parameter expansion error, this function supports multiple shells
# shellcheck disable=SC2296
# Check to ensure the expected ports are available
function _portCheck {
  local portCheckResult envVar envVarValue

  if [[ "${1-}" == "" ]] || [[ "${2-}" == "" ]]; then
    echo -e "_portCheck Usage: _portCheck <port> <portName>"
    return 0
  fi

  envVar="${1-}"
  if [[ -n "$ZSH_VERSION" ]]; then
    envVarValue="${(P)envVar}"
  elif [[ -n "$BASH_VERSION" ]]; then
    envVarValue="${!envVar}"
  else
    echo -e "$(YELLOW "Unknown/Unsupported shell, cannot verify availability of ${2-}'s intended port, proceed with caution")"
    return 0
  fi

  echo -en "Checking ${2-}'s port (${envVarValue}) "
  portCheckResult=$(lsof -w -i :"${envVarValue}" 2>&1)
  if [[ "${portCheckResult}" != "" ]]; then
      echo -e "$(RED "The intended ${2-} port (${envVarValue}) is currently being used, the process using this port should be closed or the port value should be changed.")"
      echo -e "$(RED "To use a different port, set the port value in ${envVar}")"
      echo -e "$(RED " ")"
      echo -e "$(RED "Example:")"
      echo -e "$(RED "export ${envVar}=1234")"
      echo -e "$(RED " ")"
      return 1
  else
    echo -e "$(GREEN "Open")"
  fi
  return 0
}

# A function for upgrading an existing (<=0.28.0) network to a later (>0.28.0) network
# The binary, which relies on environment variables were extensively altered and will not work on an existing network
# without migrating it first
function performMigration {
  if [[ "${1-}" == "" ]]; then
    # Check if the env file is in the expected location
    _setup_ziti_env_path
    if ! test -f "${ZITI_ENV_FILE}"; then
      echo -e "performMigration Usage: performMigration <env_file_path>"
      return 0
    fi
  else
    ZITI_ENV_FILE="${1-}"
  fi

  # Replace old Env Vars in the env file with new ones
  # NOTE: use of -i behaves differently for Mac vs Linux. -i.bak is a workaround so the command works in both OSs
  sed -i.bak 's/ZITI_CONTROLLER_HOSTNAME/ZITI_CTRL_EDGE_ADVERTISED_ADDRESS/g' "${ZITI_ENV_FILE}"
  sed -i.bak 's/ZITI_CONTROLLER_INTERMEDIATE_NAME/ZITI_PKI_CTRL_INTERMEDIATE_NAME/g' "${ZITI_ENV_FILE}"
  sed -i.bak 's/ZITI_CONTROLLER_RAWNAME/ZITI_CTRL_EDGE_ADVERTISED_ADDRESS/g' "${ZITI_ENV_FILE}"
  sed -i.bak 's/ZITI_CONTROLLER_ROOTCA_NAME/ZITI_PKI_CTRL_ROOTCA_NAME/g' "${ZITI_ENV_FILE}"
  sed -i.bak 's/ZITI_CTRL_EDGE_PORT/ZITI_CTRL_EDGE_ADVERTISED_PORT/g' "${ZITI_ENV_FILE}"
  sed -i.bak 's/ZITI_CTRL_IDENTITY_CA/ZITI_PKI_CTRL_CA/g' "${ZITI_ENV_FILE}"
  sed -i.bak 's/ZITI_CTRL_IDENTITY_CERT/ZITI_PKI_CTRL_CERT/g' "${ZITI_ENV_FILE}"
  sed -i.bak 's/ZITI_CTRL_IDENTITY_KEY/ZITI_PKI_CTRL_KEY/g' "${ZITI_ENV_FILE}"
  sed -i.bak 's/ZITI_CTRL_IDENTITY_SERVER_CERT/ZITI_PKI_CTRL_SERVER_CERT/g' "${ZITI_ENV_FILE}"
  sed -i.bak 's/ZITI_CTRL_PORT/ZITI_CTRL_ADVERTISED_PORT/g' "${ZITI_ENV_FILE}"
  sed -i.bak 's/ZITI_EDGE_CONTROLLER_HOSTNAME/ZITI_CTRL_EDGE_ADVERTISED_ADDRESS/g' "${ZITI_ENV_FILE}"
  sed -i.bak 's/ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME/ZITI_PKI_CTRL_EDGE_INTERMEDIATE_NAME/g' "${ZITI_ENV_FILE}"
  sed -i.bak 's/ZITI_EDGE_CONTROLLER_PORT/ZITI_CTRL_EDGE_ADVERTISED_PORT/g' "${ZITI_ENV_FILE}"
  sed -i.bak 's/ZITI_EDGE_CONTROLLER_RAWNAME/ZITI_CTRL_NAME/g' "${ZITI_ENV_FILE}"
  sed -i.bak 's/ZITI_EDGE_CONTROLLER_ROOTCA_NAME/ZITI_PKI_CTRL_EDGE_ROOTCA_NAME/g' "${ZITI_ENV_FILE}"
  sed -i.bak 's/ZITI_EDGE_CTRL_IDENTITY_CA/ZITI_PKI_EDGE_CA/g' "${ZITI_ENV_FILE}"
  sed -i.bak 's/ZITI_EDGE_CTRL_IDENTITY_CERT/ZITI_PKI_EDGE_CERT/g' "${ZITI_ENV_FILE}"
  sed -i.bak 's/ZITI_EDGE_CTRL_IDENTITY_KEY/ZITI_PKI_EDGE_KEY/g' "${ZITI_ENV_FILE}"
  sed -i.bak 's/ZITI_EDGE_CTRL_IDENTITY_SERVER_CERT/ZITI_PKI_EDGE_SERVER_CERT/g' "${ZITI_ENV_FILE}"
  sed -i.bak 's/ZITI_ROUTER_RAWNAME/ZITI_ROUTER_NAME/g' "${ZITI_ENV_FILE}"
  sed -i.bak 's/ZITI_PKI_OS_SPECIFIC/ZITI_PKI/g' "${ZITI_ENV_FILE}"
  sed -i.bak 's/ZITI_SIGNING_CERT/ZITI_PKI_SIGNER_CERT/g' "${ZITI_ENV_FILE}"
  sed -i.bak 's/ZITI_SIGNING_KEY/ZITI_PKI_SIGNER_KEY/g' "${ZITI_ENV_FILE}"
  sed -i.bak 's/ZITI_ROUTER_HOSTNAME/ZITI_ROUTER_ADVERTISED_ADDRESS/g' "${ZITI_ENV_FILE}"
  sed -i.bak 's/ZITI_SIGNING_ROOTCA_NAME/ZITI_PKI_SIGNER_ROOTCA_NAME/g' "${ZITI_ENV_FILE}"
  sed -i.bak 's/ZITI_SIGNING_INTERMEDIATE_NAME/ZITI_PKI_SIGNER_INTERMEDIATE_NAME/g' "${ZITI_ENV_FILE}"

  # Update environment variables if currently set
  if [[ "${ZITI_EDGE_CONTROLLER_HOSTNAME-}" != "" ]]; then export ZITI_CTRL_EDGE_ADVERTISED_ADDRESS="${ZITI_EDGE_CONTROLLER_HOSTNAME}"; fi
  if [[ "${ZITI_CONTROLLER_INTERMEDIATE_NAME-}" != "" ]]; then export ZITI_PKI_CTRL_INTERMEDIATE_NAME="${ZITI_CONTROLLER_INTERMEDIATE_NAME}"; fi
  if [[ "${ZITI_CONTROLLER_ROOTCA_NAME-}" != "" ]]; then export ZITI_PKI_CTRL_ROOTCA_NAME="${ZITI_CONTROLLER_ROOTCA_NAME}"; fi
  if [[ "${ZITI_CTRL_EDGE_PORT-}" != "" ]]; then export ZITI_CTRL_EDGE_ADVERTISED_PORT="${ZITI_CTRL_EDGE_PORT}"; fi
  if [[ "${ZITI_CTRL_IDENTITY_CA-}" != "" ]]; then export ZITI_PKI_CTRL_CA="${ZITI_CTRL_IDENTITY_CA}"; fi
  if [[ "${ZITI_CTRL_IDENTITY_CERT-}" != "" ]]; then export ZITI_PKI_CTRL_CERT="${ZITI_CTRL_IDENTITY_CERT}"; fi
  if [[ "${ZITI_CTRL_IDENTITY_KEY-}" != "" ]]; then export ZITI_PKI_CTRL_KEY="${ZITI_CTRL_IDENTITY_KEY}"; fi
  if [[ "${ZITI_CTRL_IDENTITY_SERVER_CERT-}" != "" ]]; then export ZITI_PKI_CTRL_SERVER_CERT="${ZITI_CTRL_IDENTITY_SERVER_CERT}"; fi
  if [[ "${ZITI_CTRL_PORT-}" != "" ]]; then export ZITI_CTRL_ADVERTISED_PORT="${ZITI_CTRL_PORT}"; fi
  if [[ "${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME-}" != "" ]]; then export ZITI_PKI_CTRL_EDGE_INTERMEDIATE_NAME="${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}"; fi
  if [[ "${ZITI_EDGE_CONTROLLER_RAWNAME-}" != "" ]]; then export ZITI_CTRL_NAME="${ZITI_EDGE_CONTROLLER_RAWNAME}"; fi
  if [[ "${ZITI_EDGE_CONTROLLER_ROOTCA_NAME-}" != "" ]]; then export ZITI_PKI_CTRL_EDGE_ROOTCA_NAME="${ZITI_EDGE_CONTROLLER_ROOTCA_NAME}"; fi
  if [[ "${ZITI_EDGE_CTRL_IDENTITY_CA-}" != "" ]]; then export ZITI_PKI_EDGE_CA="${ZITI_EDGE_CTRL_IDENTITY_CA}"; fi
  if [[ "${ZITI_EDGE_CTRL_IDENTITY_CERT-}" != "" ]]; then export ZITI_PKI_EDGE_CERT="${ZITI_EDGE_CTRL_IDENTITY_CERT}"; fi
  if [[ "${ZITI_EDGE_CTRL_IDENTITY_KEY-}" != "" ]]; then export ZITI_PKI_EDGE_KEY="${ZITI_EDGE_CTRL_IDENTITY_KEY}"; fi
  if [[ "${ZITI_EDGE_CTRL_IDENTITY_SERVER_CERT-}" != "" ]]; then export ZITI_ROUTER_NAME="${ZITI_EDGE_CTRL_IDENTITY_SERVER_CERT}"; fi
  if [[ "${ZITI_PKI_OS_SPECIFIC-}" != "" ]]; then export ZITI_PKI="${ZITI_PKI_OS_SPECIFIC}"; fi
  if [[ "${ZITI_SIGNING_CERT-}" != "" ]]; then export ZITI_PKI_SIGNER_CERT="${ZITI_SIGNING_CERT}"; fi
  if [[ "${ZITI_SIGNING_KEY-}" != "" ]]; then export ZITI_PKI_SIGNER_KEY="${ZITI_SIGNING_KEY}"; fi
  if [[ "${ZITI_ROUTER_HOSTNAME-}" != "" ]]; then export ZITI_ROUTER_ADVERTISED_ADDRESS="${ZITI_ROUTER_HOSTNAME}"; fi
  if [[ "${ZITI_SIGNING_ROOTCA_NAME-}" != "" ]]; then export ZITI_PKI_SIGNER_ROOTCA_NAME="${ZITI_SIGNING_ROOTCA_NAME}"; fi
  if [[ "${ZITI_SIGNING_INTERMEDIATE_NAME-}" != "" ]]; then export ZITI_PKI_SIGNER_INTERMEDIATE_NAME="${ZITI_SIGNING_INTERMEDIATE_NAME}"; fi

  # Update the necessary ziti binary references (others are not needed or are overwritten later)
  if [[ "${ZITI_BIN_DIR-}" != "" ]]; then
    sed -i.bak '/^export ZITI_BIN_DIR=/d' "${ZITI_ENV_FILE}"
    echo "export ZITI_BIN_DIR=${ZITI_BIN_DIR}" >> "${ZITI_ENV_FILE}"
  fi
  if [[ "${ZITI_BINARIES_VERSION-}" != "" ]]; then
    sed -i.bak '/^export ZITI_BINARIES_VERSION=/d' "${ZITI_ENV_FILE}"
    echo "export ZITI_BINARIES_VERSION=${ZITI_BINARIES_VERSION}" >> "${ZITI_ENV_FILE}"
  fi

  echo -e "$(GREEN "SUCCESS: Your Environment file has been updated, please use source the file for the latest values. Be sure to source the .env file as needed.")"
}

# ******* Deprecated functions, refer to new functions **********
function deprecationMessage {
  echo -e "$(YELLOW "WARNING: The ${1} function has been deprecated, please use ${2} going forward")"
}

function generateEnvFile {
  deprecationMessage generateEnvFile persistEnvironmentValues
  persistEnvironmentValues
}
function waitForController {
  deprecationMessage waitForController _wait_for_controller
  _wait_for_controller
}

function printUsage() {
    echo "Usage: ${1-} [cert to test] [ca pool to use]"
}

function verifyCertAgainstPool() {
  if [[ "" == "${1-}" ]]
  then
      printUsage "verifyCertAgainstPool"
      return 1
  fi

  if [[ "" == "$2" ]]
  then
      printUsage "verifyCertAgainstPool"
      return 1
  fi

  echo "    Verifying that this certificate:"
  echo "        - ${1-}"
  echo "    is valid for this ca pool:"
  echo "        - $2"
  echo ""
  openssl verify -partial_chain -CAfile "$2" "${1-}"
  # shellcheck disable=SC2181
  if [ $? -eq 0 ]; then
      echo ""
      echo "============      SUCCESS!      ============"
  else
      echo ""
      echo "============ FAILED TO VALIDATE ============"
  fi
}

function showIssuerAndSubjectForPEM() {
  echo "Displaying Issuer and Subject for cert pool:"
  echo "    ${1-}"
  openssl crl2pkcs7 -nocrl -certfile "${1-}" | openssl pkcs7 -print_certs -text -noout | grep -E "(Subject|Issuer)"
}
set +uo pipefail
