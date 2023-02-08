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
  local advertised_host_port="${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME}:${ZITI_CTRL_WEB_ADVERTISED_PORT}"
  while [[ "$(curl -w "%{http_code}" -m 1 -s -k -o /dev/null https://"${advertised_host_port}"/version)" != "200" ]]; do
    echo "waiting for https://${advertised_host_port}"
    sleep 3
  done
}

function _setup_ziti_home {
  _setup_ziti_network
  if [[ "${ZITI_HOME-}" == "" ]]; then export ZITI_HOME="${HOME}/.ziti/quickstart/${ZITI_NETWORK-}"; fi
}

function _setup_ziti_network {
  if [[ "${ZITI_NETWORK-}" == "" ]]; then ZITI_NETWORK="$(hostname -s)"; export ZITI_NETWORK; fi
}

function _get_file_overwrite_permission() {
  local file_path="${1-}"

  if [[ -f "${file_path}" ]]; then
    echo -en "This will overwrite the existing file, continue (y/N)? "
    read -r
    if [[ "${REPLY}" == [^Yy]* ]]; then
      echo -e "$(RED "  --- Cancelling overwrite ---")"
      return 1
    fi

    return 0
  fi
}

function _pki_client_server {
  _check_env_variable ZITI_PKI ZITI_BIN_DIR
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi
  local allow_list ZITI_CA_NAME_local ip_local file_name
  allow_list=${1-}
  ZITI_CA_NAME_local=$2
  ip_local=$3
  file_name=$4

  if [[ "${ip_local}" == "" ]]; then
    ip_local="127.0.0.1"
  fi

  if ! test -f "${ZITI_PKI}/${ZITI_CA_NAME_local}/keys/${file_name}-server.key"; then
    echo "Creating server cert from ca: ${ZITI_CA_NAME_local} for ${allow_list} / ${ip_local}"
    "${ZITI_BIN_DIR-}/ziti" pki create server --pki-root="${ZITI_PKI}" --ca-name "${ZITI_CA_NAME_local}" \
          --server-file "${file_name}-server" \
          --dns "${allow_list}" --ip "${ip_local}" \
          --server-name "${file_name} server certificate"
  else
    echo "Creating server cert from ca: ${ZITI_CA_NAME_local} for ${allow_list}"
    echo "key exists"
  fi

  if ! test -f "${ZITI_PKI}/${ZITI_CA_NAME_local}/keys/${file_name}-client.key"; then
    echo "Creating client cert from ca: ${ZITI_CA_NAME_local} for ${allow_list}"
    "${ZITI_BIN_DIR-}/ziti" pki create client --pki-root="${ZITI_PKI}" --ca-name "${ZITI_CA_NAME_local}" \
          --client-file "${file_name}-client" \
          --key-file "${file_name}-server" \
          --client-name "${file_name}"
  else
    echo "Creating client cert from ca: ${ZITI_CA_NAME_local} for ${allow_list}"
    echo "key exists"
  fi
  echo " "
}

function _pki_create_ca {
  _check_env_variable ZITI_PKI ZITI_BIN_DIR
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi
  local cert
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
  local missing_requirements=""
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
      echo "${missing_requirements}"
      echo " "
      echo " "
      return 1
  fi
}

# Disable shellcheck for parameter expansion error, this function supports multiple shells
# shellcheck disable=SC2296
# Check if an environment variable is set, if not, throw an error
function _check_env_variable() {
  local _error=false
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
  local param1="${1-}"
  for zEnvVar in $(set | grep -e "^ZITI_" | sort); do
    local envvar
    envvar="$(echo "${zEnvVar}" | cut -d '=' -f1)"
    if [[ "-s" != "${param1-}" ]]; then echo "'${param1}'unsetting [${envvar}] ${zEnvVar}"; fi
    unset "${envvar}"
  done
  # Have to explicitly unset this one (no _ZITI prefix)
  unset ZITIx_EXPRESS_COMPLETE
}

# Checks for explicit environment variables or set as defaults, also creating directories as needed
function setupEnvironment {
  # TODO: May add an else in the case the ENV var is not empty, declaring that it was overridden
  echo "Populating environment variables"
  # General Ziti Values
  _setup_ziti_network
  _setup_ziti_home
  if [[ "${ZITI_BIN_ROOT-}" == "" ]]; then export ZITI_BIN_ROOT="${ZITI_HOME}/ziti-bin"; fi

  # Get Controller Credentials
  if [[ "${ZITI_USER-}" == "" ]]; then export ZITI_USER="admin"; fi
  if [[ "${ZITI_PWD-}" == "" ]]; then
    ZITI_PWD="$(LC_ALL=C tr -dc _A-Z-a-z-0-9 < /dev/urandom | head -c32)"
    echo -en "Do you want to keep the generated admin password '$ZITI_PWD'? (Y/n) "
    # shellcheck disable=SC2162
    local pwd_reply
    read -r pwd_reply
    if [[ -z "${pwd_reply}" || ${pwd_reply} =~ [yY] ]]; then
      echo "INFO: using ZITI_PWD=${ZITI_PWD}"
    else
      echo -en "Type the preferred admin password and press <enter>"
      read -r ZITI_PWD
    fi
  fi

  # PKI Values
  if [[ "${ZITI_PKI-}" == "" ]]; then export ZITI_PKI="${ZITI_HOME}/pki"; fi
  if [[ "${ZITI_SIGNING_CERT_NAME-}" == "" ]]; then export ZITI_SIGNING_CERT_NAME="${ZITI_NETWORK}-signing"; fi
  if [[ "${ZITI_SIGNING_ROOTCA_NAME-}" == "" ]]; then export ZITI_SIGNING_ROOTCA_NAME="${ZITI_SIGNING_CERT_NAME}-root-ca"; fi
  if [[ "${ZITI_SIGNING_INTERMEDIATE_NAME-}" == "" ]]; then export ZITI_SIGNING_INTERMEDIATE_NAME="${ZITI_SIGNING_CERT_NAME}-intermediate"; fi
  if [[ "${ZITI_SIGNING_CERT}" == "" ]]; then export ZITI_SIGNING_CERT="${ZITI_PKI}/${ZITI_SIGNING_INTERMEDIATE_NAME}/certs/${ZITI_SIGNING_INTERMEDIATE_NAME}.cert"; fi
  if [[ "${ZITI_SIGNING_KEY}" == "" ]]; then export ZITI_SIGNING_KEY="${ZITI_PKI}/${ZITI_SIGNING_INTERMEDIATE_NAME}/keys/${ZITI_SIGNING_INTERMEDIATE_NAME}.key"; fi

  # Run these functions to populate other pertinent environment values
  _detect_architecture    # ZITI_ARCH
  _detect_OS              # ZITI_OSTYPE
  getLatestZitiVersion  # ZITI_BINARIES_FILE & ZITI_BINARIES_VERSION

  # Must run after the above (dependent on other variables)
  if [[ "${ZITI_BIN_DIR-}" == "" ]]; then export ZITI_BIN_DIR="${ZITI_BIN_ROOT}/ziti-${ZITI_BINARIES_VERSION}"; fi

  # Controller Values
  if [[ "${ZITI_CTRL_LISTENER_PORT-}" == "" ]]; then export ZITI_CTRL_LISTENER_PORT="6262"; fi
  if [[ "${ZITI_CTRL_MGMT_PORT-}" == "" ]]; then export ZITI_CTRL_MGMT_PORT="10000"; fi
  if [[ "${ZITI_CTRL_WEB_ADVERTISED_PORT-}" == "" ]]; then export ZITI_CTRL_WEB_ADVERTISED_PORT="1280"; fi
  if [[ "${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME-}" == "" ]]; then export ZITI_CTRL_WEB_ADVERTISED_HOSTNAME="${ZITI_NETWORK-}"; fi
  if [[ "${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME-}" == "" ]]; then export ZITI_CTRL_WEB_ADVERTISED_HOSTNAME="${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME}"; fi
  if [[ "${ZITI_CTRL_ROOTCA_NAME-}" == "" ]]; then export ZITI_CTRL_ROOTCA_NAME="${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME}-root-ca"; fi
  if [[ "${ZITI_CTRL_INTERMEDIATE_NAME-}" == "" ]]; then export ZITI_CTRL_INTERMEDIATE_NAME="${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME}-intermediate"; fi
  if [[ "${ZITI_CTRL_EDGE_ROOTCA_NAME-}" == "" ]]; then export ZITI_CTRL_EDGE_ROOTCA_NAME="${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME}-root-ca"; fi
  if [[ "${ZITI_CTRL_EDGE_INTERMEDATE_NAME-}" == "" ]]; then export ZITI_CTRL_EDGE_INTERMEDATE_NAME="${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME}-intermediate"; fi
  if [[ "${ZITI_CTRL_IDENTITY_SERVER_CERT-}" == "" ]]; then export ZITI_CTRL_IDENTITY_SERVER_CERT="${ZITI_PKI}/${ZITI_CTRL_INTERMEDIATE_NAME}/certs/${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME}-server.chain.pem"; fi
  if [[ "${ZITI_CTRL_IDENTITY_KEY-}" == "" ]]; then export ZITI_CTRL_IDENTITY_KEY="${ZITI_PKI}/${ZITI_CTRL_INTERMEDIATE_NAME}/keys/${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME}-server.key"; fi
  if [[ "${ZITI_CTRL_IDENTITY_CA-}" == "" ]]; then export ZITI_CTRL_IDENTITY_CA="${ZITI_PKI}/cas.pem"; fi
  if [[ "${ZITI_CTRL_IDENTITY_CERT-}" == "" ]]; then export ZITI_CTRL_IDENTITY_CERT="${ZITI_PKI}/${ZITI_CTRL_INTERMEDIATE_NAME}/certs/${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME}-client.cert"; fi
  if [[ "${ZITI_CTRL_WEB_IDENTITY_CERT-}" == "" ]]; then export ZITI_CTRL_WEB_IDENTITY_CERT="${ZITI_PKI}/${ZITI_CTRL_EDGE_INTERMEDATE_NAME}/certs/${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME}-client.cert"; fi
  if [[ "${ZITI_CTRL_WEB_IDENTITY_SERVER_CERT}" == "" ]]; then export ZITI_CTRL_WEB_IDENTITY_SERVER_CERT="${ZITI_PKI}/${ZITI_CTRL_EDGE_INTERMEDATE_NAME}/certs/${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME}-server.chain.pem"; fi
  if [[ "${ZITI_CTRL_WEB_IDENTITY_KEY}" == "" ]]; then export ZITI_CTRL_WEB_IDENTITY_KEY="${ZITI_PKI}/${ZITI_CTRL_EDGE_INTERMEDATE_NAME}/keys/${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME}-server.key"; fi
  if [[ "${ZITI_CTRL_WEB_IDENTITY_CA}" == "" ]]; then export ZITI_CTRL_WEB_IDENTITY_CA="${ZITI_PKI}/${ZITI_CTRL_EDGE_INTERMEDATE_NAME}/certs/${ZITI_CTRL_EDGE_INTERMEDATE_NAME}.cert"; fi

  # Router Values
  if [[ "${ZITI_EDGE_ROUTER_DISPLAY_NAME-}" == "" ]]; then ZITI_EDGE_ROUTER_DISPLAY_NAME="${ZITI_NETWORK}-edge-router"; fi
  if [[ "${ZITI_EDGE_ROUTER_PORT-}" == "" ]]; then export ZITI_EDGE_ROUTER_PORT="3022"; fi

  # Set up directories
  mkdir -p "${ZITI_BIN_ROOT}"
  mkdir -p "${ZITI_HOME}"
  mkdir -p "${ZITI_HOME}/db"

  if [[ "${ENV_FILE-}" == "" ]]; then export ENV_FILE="${ZITI_HOME}/${ZITI_NETWORK}.env"; fi

  echo -e "$(GREEN "Your OpenZiti environment has been set up successfully.")"
  echo ""
}

# Stores environment variables prefixed with ZITI_ to a .env file
function persistEnvironmentValues {
  # Get the file path
  local filepath="${1-}"
  if [[ "" == "${filepath}" ]]; then
    _check_env_variable ENV_FILE
    retVal=$?
    if [[ "${retVal}" != 0 ]]; then
      echo -e "$(RED "  --- persistEnvironment must take a parameter or have ENV_FILE set ---")"
      return 1
    else
      filepath="${ENV_FILE}"
    fi
  fi

  # Store all ZITI_ variables in the environment file, creating the directory if necessary
  mkdir -p "$(dirname "${filepath}")" && echo "" > "${filepath}"
  local envvar envval
  for zEnvVar in $(set | grep -e "^ZITI_" | sort); do
    envvar="$(echo "${zEnvVar}" | cut -d '=' -f1)"
    envval="$(echo "${zEnvVar}" | cut -d '=' -f2-100)"
    echo "export ${envvar}=\"${envval}\"" >> "${filepath}"
  done
  echo -e "A file with all pertinent environment values was created here: $(BLUE "${filepath}")"
  echo ""
}

# Clears environment variables prefixed with ZITI_, and removes ziti environment directories
function removeZitiEnvironment {
  # No need to `_check_env_variable ZITI_VERSION_OVERRIDE ZITI_BINARIES_VERSION` as this will still run if they're blank
  echo -e "$(GREEN "Clearing existing Ziti variables and continuing with express install")"

  # Check if the user chose a specific version
  local specifiedVersion=""
  if [[ "${ZITI_VERSION_OVERRIDE-}" != "" ]] && [[ "${ZITI_VERSION_OVERRIDE-}" != "${ZITI_BINARIES_VERSION-}" ]]; then
    # Don't allow overriding the version if ziti quickstart was already run, the DB may not be compatible
    echo -e "$(RED "  --- Overriding the ziti version is not supported if the version differs from one already installed. ---")"
    echo -en "Would you like to continue by using the latest version. (y/N)?"
    read -r
    echo " "
    if [[ "${REPLY}" == [Yy]* ]]; then
      unset ZITI_VERSION_OVERRIDE
    else
      return 1
    fi
  elif [[ "${ZITI_VERSION_OVERRIDE-}" != "" ]]; then
    echo -e "$(RED "  --- You have set the ZITI_VERSION_OVERRIDE value to ${ZITI_VERSION_OVERRIDE}. ---")"
    echo -en "Would you like to use this version again, choosing no will pull the latest version. (y/N)?"
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
  _check_env_variable ZITI_HOME ZITI_CTRL_WEB_ADVERTISED_HOSTNAME ZITI_BIN_DIR
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi
  local log_file="${ZITI_HOME-}/${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME}.log"
  # shellcheck disable=SC2034
  "${ZITI_BIN_DIR-}/ziti" controller run "${ZITI_HOME}/${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME}.yaml" &> "${log_file}" &
  ZITI_EXPRESS_CONTROLLER_PID=$!
  echo -e "ziti-controller started as process id: $ZITI_EXPRESS_CONTROLLER_PID. log located at: $(BLUE "${log_file}")"
}

function stopController {
  if [[ -n ${ZITI_EXPRESS_CONTROLLER_PID:-} ]]; then
    kill "$ZITI_EXPRESS_CONTROLLER_PID"
    # shellcheck disable=SC2181
    if [[ $? == 0 ]]; then
      echo "Controller stopped."
      unset ZITI_EXPRESS_CONTROLLER_PID
      return 0
    fi
  else
    echo "ERROR: you can only stop a controller process that was started with startController" >&2
    return 1
  fi
}

function startRouter {
  local log_file="${ZITI_HOME}/${ZITI_EDGE_ROUTER_DISPLAY_NAME}.log"
  "${ZITI_BIN_DIR}/ziti" router run "${ZITI_HOME}/${ZITI_EDGE_ROUTER_DISPLAY_NAME}.yaml" > "${log_file}" 2>&1 &
  ZITI_EXPRESS_EDGE_ROUTER_PID=$!
  echo -e "Express Edge Router started as process id: $ZITI_EXPRESS_EDGE_ROUTER_PID. log located at: $(BLUE "${log_file}")"
}

function stopRouter {
  if [[ -n ${ZITI_EXPRESS_EDGE_ROUTER_PID:-} ]]; then
    # shellcheck disable=SC2015
    kill "${ZITI_EXPRESS_EDGE_ROUTER_PID}" && {
      echo "INFO: stopped router"
      unset ZITI_EXPRESS_EDGE_ROUTER_PID
    } || {
      echo "ERROR: something went wrong with stopping the router(s)" >&2
      return 1
    }
  else
    echo "ERROR: you can only stop a router process that was started with startRouter" >&2
    return 1
  fi
}

# Checks all ports intended to be used in the Ziti network
function checkZitiPorts {
    local returnCnt=0
    _portCheck "ZITI_CTRL_LISTENER_PORT" "Controller"
    returnCnt=$((returnCnt + $?))
    _portCheck "ZITI_EDGE_ROUTER_PORT" "Edge Router"
    returnCnt=$((returnCnt + $?))
    _portCheck "ZITI_CTRL_WEB_ADVERTISED_PORT" "Edge Controller"
    returnCnt=$((returnCnt + $?))
    _portCheck "ZITI_CTRL_MGMT_PORT" "Controller Management Plane"
    returnCnt=$((returnCnt + $?))
    if [[ "${ZITI_EDGE_ROUTER_LISTENER_BIND_PORT-}" != "" ]]; then
      # This port can be explicitly set but is not always, only check if set
      _portCheck "ZITI_EDGE_ROUTER_LISTENER_BIND_PORT" "Router Listener Bind Port"
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
    if [ -n "${ZITI_ARCH}" ]; then return; fi
    _detect_OS
    ZITI_ARCH="amd64"
    local detected_arch
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

function hello {
  echo "Hello!"
}

# Downloads and extracts ziti binaries onto the system. The latest version is used unless ZITI_VERSION_OVERRIDE is set.
function getZiti {
  _check_env_variable ZITI_BIN_ROOT ZITI_BIN_DIR
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    # Prompt user for input or use default
    _setup_ziti_home
    local default_path="${ZITI_HOME}/ziti-bin" reply
    echo -en "The path for ziti binaries has not been set, use the default (${default_path})? (Y/n) "
    read -r reply
    if [[ -z "${reply}" || ${reply} =~ [yY] ]]; then
      echo "INFO: using the default path ${default_path}"
      ZITI_BIN_DIR="${default_path}"
    else
      echo -en "Enter the preferred path and press <enter> (the path will be created if necessary)"
      read -r ZITI_BIN_DIR
    fi
  fi

  # Check if binaries already exist, if so, skip
  # TODO: Update this to prompt, "...already exist, do you want to update..." (may instead want to check version, see if it matches latest or is version v0.0.0 to indicate dev testing)
  "${ZITI_BIN_DIR}/ziti" -v > /dev/null
  retVal=$?
  if [[ "${retVal}" == 0 ]]; then
    echo -e "Binaries exist, using existing binaries"
    echo ""
    return 0
  fi

  echo -e "Getting OpenZiti binaries"
  echo ""

  # Make the directory
  mkdir -p "${ZITI_BIN_DIR}"
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    echo -e "  * $(RED "ERROR: An error occurred generating the path (${ZITI_BIN_DIR}")"
    return 1
  fi

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
          echo -e "  * $(RED "ERROR: This version of ziti (${ZITI_VERSION_OVERRIDE}) could not be found. Please check the version and try again. The version should follow the format \"vx.x.x\".") "
          return 1
      fi
  fi

  # Where to store the ziti binaries zip
  local ziti_binaries_file_abspath
  ziti_binaries_file_abspath="${ZITI_BIN_DIR}/${ZITI_BINARIES_FILE}"
  # Check if they're already downloaded
  if ! test -f "${ZITI_BIN_DIR}/ziti"; then
    # Get the download link
    local zitidl
    zitidl="https://github.com/openziti/ziti/releases/download/${ZITI_BINARIES_VERSION-}/${ZITI_BINARIES_FILE}"
    echo -e 'Downloading '"$(BLUE "${zitidl}")"' to '"$(BLUE "${ziti_binaries_file_abspath}")"
    curl -Ls "${zitidl}" -o "${ziti_binaries_file_abspath}"
  else
    echo -e "$(YELLOW 'Already Downloaded ')""$(BLUE "${ZITI_BINARIES_FILE}")"' at: '"${ZITI_BIN_DIR}"
  fi

  # Unzip the files
  tar -xf "${ziti_binaries_file_abspath}" --directory "${ZITI_BIN_DIR}"

  # Files unzip to a /ziti subdirectory, move them into ZITI_BIN_DIR
  # Have to rename the directory first since 'ziti' matches the filename for the 'ziti' binary
  mv "${ZITI_BIN_DIR}/ziti" "${ZITI_BIN_DIR}/ziti-extract"
  mv "${ZITI_BIN_DIR}/ziti-extract"/* "${ZITI_BIN_DIR}"

  # Cleanup
  rm "${ziti_binaries_file_abspath}"      # Remove zip
  rm -rf "${ZITI_BIN_DIR}/ziti-extract"   # Remove extract folder

  # Mark the files executable
  chmod +x "${ZITI_BIN_DIR}/"*

  echo -e "$(GREEN "OpenZiti binaries ${ZITI_BINARIES_VERSION} successfully extracted to $(BLUE "${ZITI_BIN_DIR}").")"
  echo ""
}

# Create a custom PKI
function createPki {
  _check_env_variable ZITI_CTRL_ROOTCA_NAME ZITI_CTRL_EDGE_ROOTCA_NAME ZITI_SIGNING_ROOTCA_NAME \
                      ZITI_SIGNING_INTERMEDIATE_NAME ZITI_CTRL_INTERMEDIATE_NAME \
                      ZITI_CTRL_EDGE_INTERMEDATE_NAME
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi
  echo "Generating PKI"

  _pki_create_ca "${ZITI_CTRL_ROOTCA_NAME}"
  _pki_create_ca "${ZITI_CTRL_EDGE_ROOTCA_NAME}"
  _pki_create_ca "${ZITI_SIGNING_ROOTCA_NAME}"

  local ZITI_SPURIOUS_INTERMEDIATE="${ZITI_SIGNING_INTERMEDIATE_NAME}_spurious_intermediate"
  _pki_create_intermediate "${ZITI_CTRL_ROOTCA_NAME}" "${ZITI_CTRL_INTERMEDIATE_NAME}" 1
  _pki_create_intermediate "${ZITI_CTRL_EDGE_ROOTCA_NAME}" "${ZITI_CTRL_EDGE_INTERMEDATE_NAME}" 1
  _pki_create_intermediate "${ZITI_SIGNING_ROOTCA_NAME}" "${ZITI_SPURIOUS_INTERMEDIATE}" 2
  _pki_create_intermediate "${ZITI_SPURIOUS_INTERMEDIATE}" "${ZITI_SIGNING_INTERMEDIATE_NAME}" 1

  local pki_allow_list_dns
  pki_allow_list_dns="${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME},localhost,${ZITI_NETWORK}"
  if [[ "${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME}" != "" ]]; then pki_allow_list_dns="${pki_allow_list_dns},${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME}"; fi
  if [[ "${EXTERNAL_DNS}" != "" ]]; then pki_allow_list_dns="${pki_allow_list_dns},${EXTERNAL_DNS}"; fi
  local pki_allow_list_ip
  pki_allow_list_ip="127.0.0.1"
  if [[ "${ZITI_EDGE_CONTROLLER_IP_OVERRIDE}" != "" ]]; then pki_allow_list_ip="${pki_allow_list_ip},${ZITI_EDGE_CONTROLLER_IP_OVERRIDE}"; fi
  if [[ "${EXTERNAL_IP}" != "" ]]; then pki_allow_list_ip="${pki_allow_list_ip},${EXTERNAL_IP}"; fi

  _pki_client_server "${pki_allow_list_dns}" "${ZITI_CTRL_INTERMEDIATE_NAME}" "${pki_allow_list_ip}" "${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME}"
  _pki_client_server "${pki_allow_list_dns}" "${ZITI_CTRL_EDGE_INTERMEDATE_NAME}" "${pki_allow_list_ip}" "${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME}"
  echo -e "$(GREEN "PKI generated successfully")"
  echo -e ""
}

# Disable shellcheck un-passed arguments (arguments are optional)
# shellcheck disable=SC2120
# Creates a controller config file
function createControllerConfig {
  # Allow controller name to be passed in as arg
  local controller_name="${1-}"
  # If no controller name provided and env var is not set, prompt user for a controller name
  if [[ "${controller_name}" == "" ]] && [[ -z "${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME}" ]]; then
    echo -e "$(YELLOW "createControllerConfig requires a controller name to be supplied") "
    echo -en "Enter controller name: "
    read -r controller_name

    # Quit if no name is provided
    if [[ "${controller_name}" == "" ]]; then
      echo -e "$(RED "  --- Invalid controller name provided ---")"
      return 1
    fi
  # If no controller name provided and env var is set, use env var
  elif [[ "${controller_name}" == "" ]] && [[ -n "${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME}" ]]; then
    controller_name="${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME}"
  fi

  # Make sure necessary env variables are set
  # The following are used by ziti bin to generate the config so they need to be checked:
  #   ZITI_SIGNING_KEY ZITI_CTRL_WEB_IDENTITY_CERT ZITI_CTRL_WEB_IDENTITY_SERVER_CERT ZITI_CTRL_WEB_IDENTITY_KEY
  #   ZITI_CTRL_WEB_IDENTITY_CA
  _check_env_variable ZITI_CTRL_IDENTITY_SERVER_CERT ZITI_CTRL_IDENTITY_CA ZITI_SIGNING_CERT ZITI_SIGNING_KEY \
    ZITI_BIN_DIR ZITI_CTRL_WEB_IDENTITY_CERT ZITI_CTRL_WEB_IDENTITY_SERVER_CERT ZITI_CTRL_WEB_IDENTITY_KEY \
    ZITI_CTRL_WEB_IDENTITY_CA
  local retVal=$?
  if [ $retVal -ne 0 ]; then
    return 1
  fi

  # Use the current directory if none is set
  local file_path="${ZITI_HOME}"
  if [[ "${ZITI_HOME-}" == "" ]]; then file_path="."; fi

  cat "${ZITI_CTRL_IDENTITY_SERVER_CERT}" >"${ZITI_CTRL_IDENTITY_CA}"
  cat "${ZITI_SIGNING_CERT}" >>"${ZITI_CTRL_IDENTITY_CA}"
  echo -e "wrote CA file to: $(BLUE "${ZITI_CTRL_IDENTITY_CA}")"

  local output_file="${file_path}/${controller_name}.yaml"

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
  local router_name
  router_name="${1-}"
  _create_router_config "${router_name}" "private"
}

# Helper function to create a public edge router
function createEdgeRouterConfig {
  local router_name
  router_name="${1-}"
  _create_router_config "${router_name}" "public"
}

# Helper function to create a fabric router
function createFabricRouterConfig {
  local router_name
  router_name="${1-}"
  _create_router_config "${router_name}" "fabric"
}

# The main create router config function, all others point to this
function _create_router_config {
  local router_name output_file retVal default_router_name
  # Allow router name and type to be passed in as arg
  router_name="${1-}"
  router_type="${2-}"
  if [[ "${router_name}" == "" ]]; then

    # If router name is not passed as arg, prompt user for input
    echo -e "$(YELLOW "createEdgeRouterConfig requires a router name to be supplied") "
    default_router_name="${ZITI_EDGE_ROUTER_DISPLAY_NAME}"
    echo -en "Enter router name (${default_router_name}):"
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
  elif [[ "private" != "${router_type}" ]] && [[ "public" != "${router_type}" ]]; then
    echo -e "Unknown router type parameter provided, use 'fabric', 'private', or 'public'"
  fi

  # Make sure necessary env variables are set
  # The following are used by ziti bin to generate the config so they need to be checked:
  # ZITI_CTRL_WEB_ADVERTISED_HOSTNAME ZITI_CTRL_LISTENER_PORT
  _check_env_variable ZITI_HOME ZITI_BIN_DIR ZITI_CTRL_WEB_ADVERTISED_HOSTNAME ZITI_CTRL_LISTENER_PORT
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  # Use the current directory if none is set
  local file_path="${ZITI_HOME}"
  if [[ "${ZITI_HOME-}" == "" ]]; then file_path="."; fi

  local output_file="${file_path}/${router_name}.yaml"

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
  fi
  echo -e "${router_type} router configuration file written to: $(BLUE "${output_file}")"
}

# Used to create a router, router config, then enroll the router.
function addRouter {
  local router_name router_type retVal
  # Allow router name and type to be passed in as arg
  router_name="${1-}"
  router_type="${2-}"
  # If no router name provided and env var is not set, prompt user for a router name
  if [[ "${router_name}" == "" ]] && [[ -z "${ZITI_EDGE_ROUTER_DISPLAY_NAME}" ]]; then
    echo -e "$(YELLOW "addRouter requires a router name to be supplied") "
    echo -en "Enter router name: "
    read -r router_name

    # Quit if no name is provided
    if [[ "${router_name}" == "" ]]; then
      echo -e "$(RED "  --- Invalid router name provided ---")"
      return 1
    fi
  # If no router name provided and env var is set, use env var
  elif [[ "${router_name}" == "" ]] && [[ -n "${ZITI_EDGE_ROUTER_DISPLAY_NAME}" ]]; then
    router_name="${ZITI_EDGE_ROUTER_DISPLAY_NAME}"
  fi

  # Make sure necessary env variables are set
  _check_env_variable ZITI_HOME ZITI_BIN_DIR ZITI_USER ZITI_PWD
  retVal=$?
  if [ $retVal -ne 0 ]; then
    return 1
  fi

  # Create router
  # TODO: Add prompt to user to delete an existing router if it exists
  zitiLogin
  "${ZITI_BIN_DIR-}/ziti" edge delete edge-router "${router_name}"
  "${ZITI_BIN_DIR-}/ziti" edge create edge-router "${router_name}" -o "${ZITI_HOME}/${router_name}.jwt" -t -a "${router_type}"

  # Create router config
  _create_router_config "${router_name}" "${router_type}"

  # Enroll the router
  "${ZITI_BIN_DIR-}/ziti" router enroll "${ZITI_HOME}/${router_name}.yaml" --jwt "${ZITI_HOME}/${router_name}.jwt" &> "${ZITI_HOME}/${router_name}.enrollment.log"
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    echo -e "$(RED "  --- There was an error during router enrollment ---")"
    return 1
  else
    echo -e "$(GREEN "Enrollment successful")"
  fi
}

function initializeController {
  # TODO: Update this to possibly take a controller name as an argument, possibly check pwd for controller file, then default to env var values
  _setup_ziti_home
  # Make sure necessary env variables are set
  _check_env_variable ZITI_HOME ZITI_CTRL_WEB_ADVERTISED_HOSTNAME ZITI_USER ZITI_PWD ZITI_CTRL_IDENTITY_CA ZITI_BIN_DIR
  local retVal=$?
  if [ $retVal -ne 0 ]; then
    return 1
  fi

  local log_file="${ZITI_HOME-}/${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME}-init.log"
  "${ZITI_BIN_DIR-}/ziti" controller edge init "${ZITI_HOME}/${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME}.yaml" -u "${ZITI_USER-}" -p "${ZITI_PWD}" &> "${log_file}"
  echo -e "${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME} initialized. See $(BLUE "${log_file}") for details"
}

function zitiLogin {
  local advertised_host_port="${ZITI_CTRL_WEB_ADVERTISED_HOSTNAME}:${ZITI_CTRL_WEB_ADVERTISED_PORT}"
  "${ZITI_BIN_DIR-}/ziti" edge login "${advertised_host_port}" -u "${ZITI_USER-}" -p "${ZITI_PWD}" -y 2>&1
}

function expressInstall {
  # Check if expressInstall has been run before
  if [[ "" != "${ZITIx_EXPRESS_COMPLETE-}" ]]; then
    echo -e "$(RED "  --- It looks like you've run an express install in this shell already. ---")"
    echo -en "Would you like to clear existing Ziti variables and continue (y/N)? "
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
  _check_prereq curl jq
  local retVal=$?
  if [ $retVal -ne 0 ]; then
    return 1
  fi
  _issue_greeting

  echo -e "$(PURPLE "******** Setting Up Your OpenZiti Environment ********")"
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
  initializeController
  startController
  echo "waiting for the controller to come online to allow the edge router to enroll"
  _wait_for_controller

  echo -e "$(PURPLE "******** Setting Up Edge Router ********")"
  zitiLogin
  echo -e "----------  Creating an edge router policy allowing all identities to connect to routers with a $(GREEN "#public") attribute"
  "${ZITI_BIN_DIR-}/ziti" edge delete edge-router-policy allEdgeRouters > /dev/null
  "${ZITI_BIN_DIR-}/ziti" edge create edge-router-policy allEdgeRouters --edge-router-roles '#public' --identity-roles '#all' > /dev/null

  echo -e "----------  Creating a service edge router policy allowing all services to use $(GREEN "#public") edge routers"
  "${ZITI_BIN_DIR-}/ziti" edge delete service-edge-router-policy allSvcAllRouters > /dev/null
  "${ZITI_BIN_DIR-}/ziti" edge create service-edge-router-policy allSvcAllRouters --edge-router-roles '#all' --service-roles '#all' > /dev/null

  echo "USING ZITI_EDGE_ROUTER_DISPLAY_NAME: $ZITI_EDGE_ROUTER_DISPLAY_NAME"

  addRouter "${ZITI_EDGE_ROUTER_DISPLAY_NAME}" "public"

  stopController
  echo "Edge Router enrolled. Controller stopped."

}

# Gets the latest Ziti binary (the process is different for latest vs older so unfortunately two functions are needed)
function getLatestZitiVersion {
  if ! _detect_OS; then
    return 1
  fi

  _detect_architecture

  local ziti_latest
  ziti_latest=$(curl -s https://${GITHUB_TOKEN:+${GITHUB_TOKEN}@}api.github.com/repos/openziti/ziti/releases/latest)
  ZITI_BINARIES_FILE=$(echo "${ziti_latest}" | tr '\r\n' ' ' | jq -r '.assets[] | select(.name | startswith("'"ziti-${ZITI_OSTYPE}-${ZITI_ARCH}"'")) | .name')
  ZITI_BINARIES_VERSION=$(echo "${ziti_latest}" | tr '\r\n' ' ' | jq -r '.tag_name')
}

# Ensure that the version desired as specified by ZITI_VERSION_OVERRIDE exists, this returns an error in cases where
# the version doesn't exist or possibly just the version format provided in ZITI_VERSION_OVERRIDE is incorrect.
function _verify_ziti_version_exists {

  if ! setOs; then
    return 1
  fi

  _detect_architecture

  local ziticurl
  ziticurl="$(curl -s https://${GITHUB_TOKEN:+${GITHUB_TOKEN}@}api.github.com/repos/openziti/ziti/releases/tags/"${ZITI_VERSION_OVERRIDE}")"
  ZITI_BINARIES_FILE=$(echo "${ziticurl}" | tr '\r\n' ' ' | jq -r '.assets[] | select(.name | startswith("'"ziti-${ZITI_OSTYPE}-${ZITI_ARCH}"'")) | .name')
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

  echo -e "Checking ${2-}'s port (${envVarValue}) $(GREEN "Open")"
  portCheckResult=$(lsof -w -i :"${envVarValue}" 2>&1)
  if [[ "${portCheckResult}" != "" ]]; then
      echo -e "$(RED " ")"
      echo -e "$(RED "The intended ${2-} port (${envVarValue}) is currently being used, the process using this port should be closed or the port value should be changed.")"
      echo -e "$(RED "To use a different port, set the port value in ${envVar}")"
      echo -e "$(RED " ")"
      echo -e "$(RED "Example:")"
      echo -e "$(RED "export ${envVar}=1234")"
      echo -e "$(RED " ")"
      return 1
  fi
  return 0
}

set +uo pipefail