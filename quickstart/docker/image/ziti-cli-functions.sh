#!/bin/bash

set -uo pipefail

# the default ZITI_NETWORK (network name) is the short hostname
: "${DEFAULT_ZITI_NETWORK:="$(hostname -s)"}"

# shellcheck disable=SC2155
export DEFAULT_ZITI_HOME_LOCATION="${HOME}/.ziti/quickstart/${DEFAULT_ZITI_NETWORK}"

export ZITI_QUICKSTART_ENVROOT="${HOME}/.ziti/quickstart"

ASCI_WHITE='\033[01;37m'
ASCI_RESTORE='\033[0m'
ASCI_RED='\033[00;31m'
ASCI_GREEN='\033[00;32m'
ASCI_YELLOW='\033[00;33m'
ASCI_BLUE='\033[00;34m'
#ASCI_MAGENTA='\033[00;35m'
#ASCI_PURPLE='\033[00;35m'
#ASCI_CYAN='\033[00;36m'
#ASCI_LIGHTGRAY='\033[00;37m'
#ASCI_LRED='\033[01;31m'
#ASCI_LGREEN='\033[01;32m'
#ASCI_LYELLOW='\033[01;33m'
#ASCI_LBLUE='\033[01;34m'
#ASCI_LMAGENTA='\033[01;35m'
#ASCI_LPURPLE='\033[01;35m'
#ASCI_LCYAN='\033[01;36m'

function WHITE {
  echo "${ASCI_WHITE}${1-}${ASCI_RESTORE}"
}
function RED {
  echo "${ASCI_RED}${1-}${ASCI_RESTORE}"
}
function GREEN {
  echo "${ASCI_GREEN}${1-}${ASCI_RESTORE}"
}
function YELLOW {
  echo "${ASCI_YELLOW}${1-}${ASCI_RESTORE}"
}
function BLUE {
  echo "${ASCI_BLUE}${1-}${ASCI_RESTORE}"
}

function zitiLogin {
  "${ZITI_BIN_DIR-}/ziti" edge login "${ZITI_EDGE_CTRL_ADVERTISED}" -u "${ZITI_USER-}" -p "${ZITI_PWD}" -c "${ZITI_PKI_OS_SPECIFIC}/${ZITI_EDGE_CONTROLLER_ROOTCA_NAME}/certs/${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}.cert"
}
function cleanZitiController {
  checkEnvVariable ZITI_HOME
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi
  rm -rf "${ZITI_HOME}/db"
  mkdir "${ZITI_HOME}/db"
  initializeController
}
function initializeController {
  log_file="${ZITI_HOME-}/${ZITI_EDGE_CONTROLLER_RAWNAME}-init.log"
  "${ZITI_BIN_DIR-}/ziti-controller" edge init "${ZITI_HOME_OS_SPECIFIC}/${ZITI_EDGE_CONTROLLER_RAWNAME}.yaml" -u "${ZITI_USER-}" -p "${ZITI_PWD}" &> "${log_file}"
  echo -e "ziti-controller initialized. see $(BLUE "${log_file}") for details"
}
function startController {
  log_file="${ZITI_HOME-}/${ZITI_EDGE_CONTROLLER_RAWNAME}.log"
  # shellcheck disable=SC2034
  "${ZITI_BIN_DIR-}/ziti-controller" run "${ZITI_HOME_OS_SPECIFIC}/${ZITI_EDGE_CONTROLLER_RAWNAME}.yaml" &> "${log_file}" &
  ZITI_EXPRESS_CONTROLLER_PID=$!
  echo -e "ziti-controller started as process id: $ZITI_EXPRESS_CONTROLLER_PID. log located at: $(BLUE "${log_file}")"
}

function stopController {
  if [[ -n ${ZITI_EXPRESS_CONTROLLER_PID:-} ]]; then
    kill "$ZITI_EXPRESS_CONTROLLER_PID"
    # shellcheck disable=SC2181
    if [[ $? == 0 ]]; then 
      echo "Controller stopped."
      return 0
    fi
  else
    echo "ERROR: you can only stop a controller process that was started with startController" >&2
    return 1
  fi
}

function startRouter {
  log_file="${ZITI_HOME_OS_SPECIFIC}/${ZITI_EDGE_ROUTER_RAWNAME}.log"
  "${ZITI_BIN_DIR}/ziti-router" run "${ZITI_HOME_OS_SPECIFIC}/${ZITI_EDGE_ROUTER_RAWNAME}.yaml" > "${log_file}" 2>&1 &
  ZITI_EXPRESS_EDGE_ROUTER_PID=$!
  echo -e "Express Edge Router started as process id: $ZITI_EXPRESS_EDGE_ROUTER_PID. log located at: $(BLUE "${log_file}")"
  
}

function stopRouter {
  if [[ -n ${ZITI_EXPRESS_EDGE_ROUTER_PID:-} ]]; then
    # shellcheck disable=SC2015
    kill "${ZITI_EXPRESS_EDGE_ROUTER_PID}" && {
      echo "INFO: stopped router"
    } || {
      echo "ERROR: something went wrong with stopping the router(s)" >&2
      return 1
    }
  else
    echo "ERROR: you can only stop a router process that was started with startRouter" >&2
    return 1
  fi
}

function checkHostsFile {
  ctrlexists=$(grep -c "${ZITI_CONTROLLER_HOSTNAME}" /etc/hosts)
  edgectrlexists=$(grep -c "${ZITI_EDGE_CONTROLLER_HOSTNAME}" /etc/hosts)
  erexists=$(grep -c "${ZITI_EDGE_ROUTER_HOSTNAME}" /etc/hosts)

  if [[ "0" = "${ctrlexists}" ]] || [[ "0" = "${edgectrlexists}" ]] || [[ "0" = "${erexists}" ]]; then
    echo " "
    echo -e "$(YELLOW "Ziti is generally used to create an overlay network. Generally speaking this will involve more than one host")"
    echo -e "$(YELLOW "Since this is a script geared towards setting up a very minimal development environment it needs to make some")"
    echo -e "$(YELLOW "assumptions. One of these assumptions is that the three specific entries are entered onto your hosts file.")"
    echo -e "$(YELLOW "One or more of these are missing:")"
    echo " "
    if [[ "0" == "${ctrlexists}" ]]; then
      echo -e "  * $(RED "MISSING: ${ZITI_EDGE_CONTROLLER_HOSTNAME}") "
    else
      echo -e "  * $(GREEN "  FOUND: ${ZITI_EDGE_CONTROLLER_HOSTNAME}") "
    fi
    if [[ "0" == "${edgectrlexists}" ]]; then
      echo -e "  * $(RED "MISSING: ${ZITI_EDGE_CONTROLLER_HOSTNAME}") "
    else
      echo -e "  * $(GREEN "  FOUND: ${ZITI_EDGE_CONTROLLER_HOSTNAME}") "
    fi
    if [[ "0" == "${erexists}" ]]; then
      echo -e "  * $(RED "MISSING: ${ZITI_EDGE_ROUTER_HOSTNAME}") "
    else
      echo -e "  * $(GREEN "  FOUND: ${ZITI_EDGE_ROUTER_HOSTNAME}") "
    fi

    echo " "
    echo "The easiest way to correct this is to run the following command:"
    echo "  echo \"127.0.0.1 ${ZITI_CONTROLLER_HOSTNAME} ${ZITI_EDGE_CONTROLLER_HOSTNAME} ${ZITI_EDGE_ROUTER_HOSTNAME}\" | sudo tee -a /etc/hosts"
    echo " "
    echo "add these entries to your hosts file, and rerun the script when ready"
    return 1
  else
    echo -e "$(GREEN "Hosts file looks good")"
  fi
}

function verifyZitiVersionExists {
  setupZitiHome

  if ! setOs; then
    return 1
  fi

  detectArchitecture

  unset ZITI_BINARIES_VERSION

  ziticurl="$(curl -s https://${GITHUB_TOKEN:+${GITHUB_TOKEN}@}api.github.com/repos/openziti/ziti/releases/tags/"${ZITI_VERSION_OVERRIDE}")"
  # shellcheck disable=SC2155
  export ZITI_BINARIES_FILE=$(echo "${ziticurl}" | tr '\r\n' ' ' | jq -r '.assets[] | select(.name | startswith("'"ziti-${ZITI_OSTYPE}-${ZITI_ARCH}"'")) | .name')
  # shellcheck disable=SC2155
  export ZITI_BINARIES_VERSION=$(echo "${ziticurl}" | tr '\r\n' ' ' | jq -r '.tag_name')

  # Check if there was an error while trying to get the requested version
  if [[ "${ZITI_BINARIES_VERSION-}" == "null" ]]; then
    echo "ERROR: response missing '.tag_name': ${ziticurl}" >&2
    return 1
  fi

  echo "ZITI_BINARIES_VERSION: ${ZITI_BINARIES_VERSION}"
}

function detectArchitecture {
    if [ -n "${ZITI_ARCH}" ]; then return; fi
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

function getLatestZitiVersion {
  setupZitiHome

  if ! setOs; then
    return 1
  fi

  detectArchitecture

  unset ZITI_BINARIES_VERSION

  if [[ "${ZITI_BINARIES_VERSION-}" == "" ]]; then
    zitilatest=$(curl -s https://${GITHUB_TOKEN:+${GITHUB_TOKEN}@}api.github.com/repos/openziti/ziti/releases/latest)
    # shellcheck disable=SC2155
    export ZITI_BINARIES_FILE=$(echo "${zitilatest}" | tr '\r\n' ' ' | jq -r '.assets[] | select(.name | startswith("'"ziti-${ZITI_OSTYPE}-${ZITI_ARCH}"'")) | .name')
    # shellcheck disable=SC2155
    export ZITI_BINARIES_VERSION=$(echo "${zitilatest}" | tr '\r\n' ' ' | jq -r '.tag_name')
  fi
  echo "ZITI_BINARIES_VERSION: ${ZITI_BINARIES_VERSION}"
}

function getZiti {
  setupZitiHome
  checkEnvVariable ZITI_HOME
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  ziti_bin_root="${ZITI_BIN_ROOT-}"
  if [[ "${ziti_bin_root}" == "" ]]; then
    ziti_bin_root="${ZITI_HOME-}/ziti-bin"
  fi
  export ZITI_BIN_ROOT="${ziti_bin_root}/ziti-bin"

  mkdir -p "${ziti_bin_root}"

  # Get the latest version unless a specific version is specified
  if [[ "${ZITI_VERSION_OVERRIDE-}" == "" ]]; then
    if ! getLatestZitiVersion; then
      return 1
    fi
  else
      # Check if an error occurred while trying to pull desired version (happens with incorrect version or formatting issue)
      if ! verifyZitiVersionExists; then
          echo -e "  * $(RED "ERROR: This version of ziti (${ZITI_VERSION_OVERRIDE}) could not be found. Please check the version and try again. The version should follow the format \"vx.x.x\".") "
          return 1
      fi
  fi

  if [[ "${ZITI_BIN_DIR-}" == "" ]]; then export ZITI_BIN_DIR="${ziti_bin_root}/ziti-${ZITI_BINARIES_VERSION}"; else echo "Using ZITI_BIN_DIR: ${ZITI_BIN_DIR}"; fi

  ZITI_BINARIES_FILE_ABSPATH="${ziti_bin_root}/${ZITI_BINARIES_FILE}"
  if ! test -f "${ZITI_BINARIES_FILE_ABSPATH}"; then
    zitidl="https://github.com/openziti/ziti/releases/download/${ZITI_BINARIES_VERSION-}/${ZITI_BINARIES_FILE}"
    echo -e 'Downloading '"$(BLUE "${zitidl}")"' to '"$(BLUE "${ZITI_BINARIES_FILE_ABSPATH}")"
    curl -Ls "${zitidl}" -o "${ZITI_BINARIES_FILE_ABSPATH}"
  else
    echo -e "$(YELLOW 'Already Downloaded ')""$(BLUE "${ZITI_BINARIES_FILE}")"' at: '"${ZITI_BINARIES_FILE_ABSPATH}"
  fi

  echo -e 'UNZIPPING '"$(BLUE "${ZITI_BINARIES_FILE_ABSPATH}")"' into: '"$(GREEN "${ZITI_BIN_DIR}")"
  rm -rf "${ziti_bin_root}/ziti-${ZITI_BINARIES_VERSION-}"
  if [[ "${ZITI_OSTYPE}" == "windows" ]]; then
    unzip "${ZITI_BINARIES_FILE_ABSPATH}" -d "${ZITI_BIN_DIR}"
    mv  "${ZITI_BIN_DIR}/ziti/"*  "${ZITI_BIN_DIR}/"
    rm -rf "${ZITI_BIN_DIR}/ziti/"*
    rmdir "${ZITI_BIN_DIR}/ziti/"
    chmod +x "${ZITI_BIN_DIR}/"*
  else
    tar -xf "${ZITI_BINARIES_FILE_ABSPATH}" --directory "${ziti_bin_root}"
    mv "${ziti_bin_root}/ziti" "${ZITI_BIN_DIR}"
  fi

  echo -e 'Marking executables at '"$(GREEN "${ZITI_BIN_DIR}")"' executable'
  chmod +x "${ZITI_BIN_DIR}/"*

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

function checkPrereqs {
  commands_to_test=(curl jq)
  missing_requirements=""
  # verify all the commands required in the automation exist before trying to run the full suite
  for cmd in "${commands_to_test[@]}"
  do
      # checking all commands are on the path before continuing...
      if ! [[ -x "$(command -v "${cmd}")" ]]; then
          missing_requirements="${missing_requirements}    * ${cmd}"
      fi
  done
  # are requirements ? if yes, stop here and help 'em out
  if ! [[ "" = "${missing_requirements}" ]]; then
      echo " "
      echo "You're missing one or more commands that are used in this script."
      echo "Please ensure the commands listed are on the path and then try again."
      echo "${missing_requirements}"
      echo " "
      echo " "
      return 1
  else
    echo -e "$(GREEN "Prerequisites confirmed")"
  fi
}

function _portCheck {
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

  echo -e "Checking if ${2-}'s port (${envVarValue}) is available"
  portCheckResult=$(lsof -w -i :"${envVarValue}" 2> /dev/null)
  if [[ "${portCheckResult}" != "" ]]; then # Controller management plane
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

function checkControllerName {
  if [[ "${ZITI_EDGE_CONTROLLER_HOSTNAME}" == *['!'@#\$%^\&*\(\)_+]* ]]; then
    echo -e "$(RED "  - The provided Network name contains an invalid character: '!'@#\$%^\&*()_+")"
    return 1
  fi
  return 0
}

function unsetZitiEnv {
  if [[ "${1-}" == "-s" ]]; then
    # Silent...
    for zEnvVar in $(set | grep -e "^ZITI_" | sort); do envvar="$(echo "${zEnvVar}" | cut -d '=' -f1)"; unset "${envvar}"; done
  else
    for zEnvVar in $(set | grep -e "^ZITI_" | sort); do envvar="$(echo "${zEnvVar}" | cut -d '=' -f1)"; echo unsetting "[${envvar}]${zEnvVar}"; unset "${envvar}"; done
  fi
  unset ZITIx_EXPRESS_COMPLETE
}

function issueGreeting {
  #echo "-------------------------------------------------------------"
  echo " "
  echo "                          _   _     _"
  echo "                    ____ (_) | |_  (_)"
  echo "                   |_  / | | | __| | |"
  echo "                    / /  | | | |_  | |"
  echo "                   /___| |_|  \__| |_|"
  echo "-------------------------------------------------------------"
  echo " "
  echo "This script will make it trivial to setup a very simple environment locally which will allow you to start"
  echo "learning ziti. This environment is suitable for development work only and is not a decent representation of"
  echo "a fully redundant production-caliber network."
  echo ""
  echo "Please note that this script will write files to your home directory into a directory named .ziti."
  echo -n "For you this location will be: "
  echo -e "$(BLUE "${ZITI_QUICKSTART_ENVROOT}")"
  echo " "
}

function setupZitiNetwork {
  if [[ "${1-}" == "" ]]; then
    echo " "
    echo "Creating a controller is effectively creating a network. The name of the network will be used when writing"
    echo "configuration files locally. Choose the name of your network now. The format of the network name should resemble"
    echo -n "what a hostname looks like. A good choice is to actually use your system's hostname: "
    echo -e "$(BLUE "${DEFAULT_ZITI_NETWORK}")"
    echo " "
    echo -en "$(echo -ne "Network Name [$(BLUE "${DEFAULT_ZITI_NETWORK}")]: ")"
    read -r ZITI_NETWORK
    echo " "
    if checkControllerName; then
      : #clear to continue
      if [[ "${ZITI_NETWORK-}" == "" ]]; then
        ZITI_NETWORK="${DEFAULT_ZITI_NETWORK}"
      fi
      echo "name: ${ZITI_NETWORK-}"
    else
      echo ""
      echo "============ FAILED TO SETUP NETWORK ============"
      return 1
    fi
    echo " "
  else
    ZITI_NETWORK="${1-}"
  fi
}

function setupZitiHome {
  if [[ "${ZITI_HOME-}" == "" ]]; then
    export ZITI_HOME="${HOME}/.ziti/quickstart/${ZITI_NETWORK-}"
    echo -e "using default ZITI_HOME: $(BLUE "${ZITI_HOME}")"
  fi
}

function generateEnvFile {

  echo -e "Generating new network with name: $(BLUE "${ZITI_NETWORK-}")"

  if [[ "${ZITI_CONTROLLER_RAWNAME-}" == "" ]]; then export ZITI_CONTROLLER_RAWNAME="${ZITI_NETWORK-}"; fi
  if [[ "${ZITI_CONTROLLER_HOSTNAME-}" == "" ]]; then export ZITI_CONTROLLER_HOSTNAME="${ZITI_CONTROLLER_RAWNAME}"; fi
  if [[ "${ZITI_CTRL_PORT-}" == "" ]]; then export ZITI_CTRL_PORT="6262"; fi
  if [[ "${ZITI_EDGE_ROUTER_PORT-}" == "" ]]; then export ZITI_EDGE_ROUTER_PORT="3022"; fi

  if [[ "${ZITI_EDGE_CONTROLLER_RAWNAME-}" == "" ]]; then export ZITI_EDGE_CONTROLLER_RAWNAME="${ZITI_NETWORK-}"; fi
  if [[ "${ZITI_EDGE_CONTROLLER_HOSTNAME-}" == "" ]]; then export ZITI_EDGE_CONTROLLER_HOSTNAME="${ZITI_EDGE_CONTROLLER_RAWNAME}"; fi

  if [[ "${ZITI_ZAC_RAWNAME-}" == "" ]]; then export ZITI_ZAC_RAWNAME="${ZITI_NETWORK-}"; fi
  if [[ "${ZITI_ZAC_HOSTNAME-}" == "" ]]; then export ZITI_ZAC_HOSTNAME="${ZITI_ZAC_RAWNAME}"; fi

  if [[ "${ZITI_BIN_ROOT-}" == "" ]]; then
    export ZITI_BIN_ROOT="${ZITI_HOME-}/ziti-bin"
  fi

  export ENV_FILE="${ZITI_HOME-}/${ZITI_NETWORK-}.env"

  if ! ziti_createEnvFile; then
    return 1
  fi

  echo -e "environment file sourced from: $(BLUE "${ENV_FILE}")"
}

function ziti_expressConfiguration {
  if [[ "${ZITIx_EXPRESS_COMPLETE-}" != "" ]]; then
    echo -e "$(RED "  --- It looks like you've run an express install in this shell already. ---")"
    echo -en "Would you like to clear existing Ziti variables and continue (y/N)? "
    read -r
    echo " "
    if [[ "${REPLY}" == [Yy]* ]]; then
      echo -e "$(GREEN "Clearing existing Ziti variables and continuing with express install")"

      # Check if the user chose a specific version
      specifiedVersion=""
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
      
      # Silently clear ziti variables
      unsetZitiEnv "-s"

    else
      echo -e "$(RED "  --- Exiting express install ---")"
      return 1
    fi
  fi
  export ZITIx_EXPRESS_COMPLETE="true"
  echo " "
  echo "___________   _______________________________________^__"
  echo " ___   ___ |||  ___   ___   ___    ___ ___  |   __  ,----\ "
  echo "|   | |   |||| |   | |   | |   |  |   |   | |  |  | |_____\ "
  echo "|___| |___|||| |___| |___| |___|  | O | O | |  |  |        \ "
  echo "           ||| ===== EXPRESS ==== |___|___| |  |__|         )"
  echo "___________|||______________________________|______________/"
  echo "           |||                                        /--------"
  echo "-----------'''---------------------------------------'"
  echo ""
  if [[ "${ZITI_CONTROLLER_RAWNAME-}" != "" ]]; then echo "ZITI_CONTROLLER_RAWNAME OVERRIDDEN: $ZITI_CONTROLLER_RAWNAME"; fi
  if [[ "${ZITI_CONTROLLER_HOSTNAME-}" != "" ]]; then echo "ZITI_CONTROLLER_HOSTNAME OVERRIDDEN: $ZITI_CONTROLLER_HOSTNAME"; fi
  if [[ "${ZITI_EDGE_CONTROLLER_RAWNAME-}" != "" ]]; then echo "ZITI_EDGE_CONTROLLER_RAWNAME OVERRIDDEN: $ZITI_EDGE_CONTROLLER_RAWNAME"; fi
  if [[ "${ZITI_EDGE_CONTROLLER_HOSTNAME-}" != "" ]]; then echo "ZITI_EDGE_CONTROLLER_HOSTNAME OVERRIDDEN: $ZITI_EDGE_CONTROLLER_HOSTNAME"; fi
  if [[ "${ZITI_ZAC_RAWNAME-}" != "" ]]; then echo "ZITI_ZAC_RAWNAME OVERRIDDEN: $ZITI_ZAC_RAWNAME"; fi
  if [[ "${ZITI_ZAC_HOSTNAME-}" != "" ]]; then echo "ZITI_ZAC_HOSTNAME OVERRIDDEN: $ZITI_ZAC_HOSTNAME"; fi
  if [[ "${ZITI_EDGE_ROUTER_HOSTNAME-}" != "" ]]; then echo "ZITI_EDGE_ROUTER_HOSTNAME OVERRIDDEN: $ZITI_EDGE_ROUTER_HOSTNAME"; fi
  if [[ "${ZITI_EDGE_ROUTER_PORT-}" != "" ]]; then echo "ZITI_EDGE_ROUTER_PORT OVERRIDDEN: $ZITI_EDGE_ROUTER_PORT"; fi
  if [[ "${ZITI_EDGE_ROUTER_RAWNAME-}" != "" ]]; then echo "ZITI_EDGE_ROUTER_RAWNAME OVERRIDDEN: $ZITI_EDGE_ROUTER_RAWNAME"; fi

  echo " "
  echo " "
  echo -e "******** Setting Up Environment ********"
  if [[ "${1-}" == "" ]]; then
    nw="${DEFAULT_ZITI_NETWORK}"
  else
    nw="${1-}"
  fi

  setupZitiNetwork "${nw}"
  setupZitiHome
  echo -e "ZITI HOME SET TO: $(BLUE "${ZITI_HOME}")"

  if ! getZiti "no"; then
    echo -e "$(RED "getZiti failed")"
    return 1
  fi
  if ! generateEnvFile; then
    echo "Exiting as env file was not generated"
    return 1
  fi

  # Check Ports
  returnCnt=0
  _portCheck "ZITI_CTRL_PORT" "Controller"
  returnCnt=$((returnCnt + $?))
  _portCheck "ZITI_EDGE_ROUTER_PORT" "Edge Router"
  returnCnt=$((returnCnt + $?))
  _portCheck "ZITI_EDGE_CONTROLLER_PORT" "Edge Controller"
  returnCnt=$((returnCnt + $?))
  _portCheck "ZITI_CTRL_MGMT_PORT" "Controller Management Plane"
  returnCnt=$((returnCnt + $?))
  if [[ "${ZITI_EDGE_ROUTER_LISTENER_BIND_PORT-}" != "" ]]; then
    # This port can be explicitly set but is not always, only check if set
    _portCheck "ZITI_EDGE_ROUTER_LISTENER_BIND_PORT" "Router Listener Bind Port"
    returnCnt=$((returnCnt + $?))
  fi
  if [[ "returnCnt" -gt "0" ]]; then return 1; fi

  #checkHostsFile

  echo " "
  echo " "
  echo -e "******** Setting Up Public Key Infrastructure ********"
  createPki

  echo " "
  echo " "
  echo -e "******** Setting Up Controller ********"
  createControllerConfig
  #createControllerSystemdFile
  initializeController
  startController
  echo "waiting for the controller to come online to allow the edge router to enroll"

  waitForController

  zitiLogin

  echo " "
  echo " "
  echo -e "******** Setting Up Edge Routers ********"
  echo -e "----------  Creating an edge router policy allowing all identities to connect to routers with a $(GREEN "#public") attribute"
  "${ZITI_BIN_DIR-}/ziti" edge delete edge-router-policy allEdgeRouters > /dev/null
  "${ZITI_BIN_DIR-}/ziti" edge create edge-router-policy allEdgeRouters --edge-router-roles '#public' --identity-roles '#all' > /dev/null

  echo -e "----------  Creating a service edge router policy allowing all services to use $(GREEN "#public") edge routers"
  "${ZITI_BIN_DIR-}/ziti" edge delete service-edge-router-policy allSvcAllRouters > /dev/null
  "${ZITI_BIN_DIR-}/ziti" edge create service-edge-router-policy allSvcAllRouters --edge-router-roles '#all' --service-roles '#all' > /dev/null

  echo "USING ZITI_EDGE_ROUTER_RAWNAME: $ZITI_EDGE_ROUTER_RAWNAME"

  createRouterPki "${ZITI_EDGE_ROUTER_RAWNAME}"

  createEdgeRouterConfig "${ZITI_EDGE_ROUTER_RAWNAME}"
  #createRouterSystemdFile "${ZITI_EDGE_ROUTER_RAWNAME}"

  echo "----------  Creating edge-router ${ZITI_EDGE_ROUTER_RAWNAME}...."
  "${ZITI_BIN_DIR-}/ziti" edge delete edge-router "${ZITI_EDGE_ROUTER_RAWNAME}" > /dev/null
  "${ZITI_BIN_DIR-}/ziti" edge create edge-router "${ZITI_EDGE_ROUTER_RAWNAME}" -o "${ZITI_HOME_OS_SPECIFIC}/${ZITI_EDGE_ROUTER_RAWNAME}.jwt" -t -a "public"> /dev/null
  sleep 1
  echo "----------  Enrolling edge-router ${ZITI_EDGE_ROUTER_RAWNAME}...."

  "${ZITI_BIN_DIR-}/ziti-router" enroll "${ZITI_HOME_OS_SPECIFIC}/${ZITI_EDGE_ROUTER_RAWNAME}.yaml" --jwt "${ZITI_HOME_OS_SPECIFIC}/${ZITI_EDGE_ROUTER_RAWNAME}.jwt" &> "${ZITI_HOME_OS_SPECIFIC}/${ZITI_EDGE_ROUTER_RAWNAME}.enrollment.log"
  echo ""

  stopController
  echo "Edge Router enrolled. Controller stopped."

  echo ""
  echo -e "$(GREEN "Congratulations. Express setup complete!")"
  echo -e "Start your Ziti Controller by running the function: $(BLUE "startController")"
  echo -e "Start your Ziti Edge Router by running : $(BLUE 'startRouter')"
  echo ""
}

function decideToUseDefaultZitiHome {
  yn=""
  while true
  do
    echo "ZITI_HOME has not been set. Do you want to use the default ZITI_HOME: ${DEFAULT_ZITI_HOME_LOCATION}"
    echo " "
    echo -en "Select an action: "
    read -r yn
    case $yn in
        [yY]* )
            ZITI_HOME="${DEFAULT_ZITI_HOME_LOCATION}"
            break;;
        [Nn]* )
            echo ""
            return 1;;
        * )
            echo " "
            echo "Answer $yn is not valid. Please answer yes or no. (y/n [yes/NO])";;
    esac
    yn=
  done
  echo " "
}

function decideOperation {
  yn="${1-}"
  while true
  do
    if [[ "${yn}" == "" ]]; then
      echo "What sort of operation are you looking to perform?"
      echo "  1.) Express configuration - a simple overlay will be spawned containing one controller and two edge routers"
      echo "  2.) Create Controller configuration - answer a few questions and a controller config will be emitted"
      echo "  3.) Create Edge Router configuration - answer a few questions and an edge router config will be emitted"
      echo "  4.) Start a network with the provided name"
      echo " "
      echo -en "Select an action: "
      read -r yn
    fi
    case $yn in
        [1]* )
            ziti_expressConfiguration "$2"
            break;;
        [2]* )
            generateEnvFile

            ;;
        [3]* )
            echo "333 has been chosen"
            echo " "
            ;;
        [4]* )
            echo "4444 has been chosen"
            echo " "
            ;;
        [yYqQeE]* )
            break
            ;;
        [Nn]* )
            echo ""; echo "Ok - come back when you're ready."
            exit;;
        * ) echo "Please answer yes or no. (yes/NO)";;
    esac
    yn=
  done
  echo " "
}

function expressInstall {
  #greet the user with the banner and quick blurb about what to expect
  issueGreeting

  #make sure the user has all the necessary commands to be successful
  checkPrereqs
  retval=$?
  if [ $retval -ne 0 ]; then
    return 1
  fi
  echo "Let's get started creating your local development network!"
  echo ""
  echo ""

  #prompt the user for input and do what they want/need
  decideOperation 1 "${1-}"
}

function pki_client_server {
  allow_list=${1-}
  ZITI_CA_NAME_local=$2
  ip_local=$3
  file_name=$4

  if [[ "${ip_local}" == "" ]]; then
    ip_local="127.0.0.1"
  fi

  if ! test -f "${ZITI_PKI}/${ZITI_CA_NAME_local}/keys/${file_name}-server.key"; then
    echo "Creating server cert from ca: ${ZITI_CA_NAME_local} for ${allow_list} / ${ip_local}"
    "${ZITI_BIN_DIR-}/ziti" pki create server --pki-root="${ZITI_PKI_OS_SPECIFIC}" --ca-name "${ZITI_CA_NAME_local}" \
          --server-file "${file_name}-server" \
          --dns "${allow_list}" --ip "${ip_local}" \
          --server-name "${file_name} server certificate"
  else
    echo "Creating server cert from ca: ${ZITI_CA_NAME_local} for ${allow_list}"
    echo "key exists"
  fi

  if ! test -f "${ZITI_PKI}/${ZITI_CA_NAME_local}/keys/${file_name}-client.key"; then
    echo "Creating client cert from ca: ${ZITI_CA_NAME_local} for ${allow_list}"
    "${ZITI_BIN_DIR-}/ziti" pki create client --pki-root="${ZITI_PKI_OS_SPECIFIC}" --ca-name "${ZITI_CA_NAME_local}" \
          --client-file "${file_name}-client" \
          --key-file "${file_name}-server" \
          --client-name "${file_name}"
  else
    echo "Creating client cert from ca: ${ZITI_CA_NAME_local} for ${allow_list}"
    echo "key exists"
  fi
  echo " "
}

function pki_create_ca {
  cert=$1

  echo "Creating CA: ${cert}"
  if ! test -f "${ZITI_PKI}/${cert}/keys/${cert}.key"; then
    "${ZITI_BIN_DIR}/ziti" pki create ca --pki-root="${ZITI_PKI_OS_SPECIFIC}" --ca-file="${cert}" --ca-name="${cert} Root CA"
  else
    echo "key exists"
  fi
  echo " "
}

function pki_create_intermediate {
  echo "Creating intermediate: ${1} ${2} ${3}"
  if ! test -f "${ZITI_PKI}/${2}/keys/${2}.key"; then
    "${ZITI_BIN_DIR}/ziti" pki create intermediate --pki-root "${ZITI_PKI_OS_SPECIFIC}" --ca-name "${1}" \
          --intermediate-name "${2}" \
          --intermediate-file "${2}" --max-path-len "${3}"
  else
    echo "key exists"
  fi
  echo " "
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

function createRouterPki {
  # Allow router name to be passed in as arg
  router_name="${1-}"
  if [[ "${router_name}" == "" ]]; then
    echo -e "  * ERROR: $(RED "routerName needs to be supplied") "
    return 1
  fi
  mkdir -p "${ZITI_PKI_OS_SPECIFIC}/routers/${router_name}"
  export ZITI_ROUTER_IDENTITY_CERT="${ZITI_PKI_OS_SPECIFIC}/routers/${router_name}/client.cert"
  export ZITI_ROUTER_IDENTITY_SERVER_CERT="${ZITI_PKI_OS_SPECIFIC}/routers/${router_name}/server.cert"
  export ZITI_ROUTER_IDENTITY_KEY="${ZITI_PKI_OS_SPECIFIC}/routers/${router_name}/server.key"
  export ZITI_ROUTER_IDENTITY_CA="${ZITI_PKI_OS_SPECIFIC}/routers/${router_name}/cas.cert"
  pki_client_server "${router_name},localhost,127.0.0.1,${ZITI_NETWORK}" "${ZITI_CONTROLLER_INTERMEDIATE_NAME}" "${ZITI_EDGE_ROUTER_IP_OVERRIDE-}" "${router_name}"
}

function createPrivateRouterConfig {
  # Allow router name to be passed in as arg
  router_name="${1-}"
  if [[ "${router_name}" == "" ]]; then

    # If router name is not passed as arg, prompt user for input
    echo -e "$(YELLOW "createPrivateRouterConfig requires a router name to be supplied") "
    default_router_name="${ZITI_EDGE_ROUTER_RAWNAME}"
    echo -en "Enter router name (${default_router_name}):"
    read -r router_name

    # Accept the default if no name provided
    if [[ "${router_name}" == "" ]]; then
      # Check for overwrite of default file
      router_name="${default_router_name}"
      if [[ -f "${ZITI_HOME-}/${router_name}.yaml" ]]; then
        echo -en "This will overwrite the existing file, continue (y/N)? "
        read -r
        echo " ${REPLY}"
        if [[ "${REPLY}" == [^Yy]* ]]; then
          echo -e "$(RED "  --- Cancelling overwrite ---")"
          return 1
        fi
      fi
    fi
  fi

  # Make sure necessary env variables are set
  checkEnvVariable ZITI_HOME
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  output_file="${ZITI_HOME-}/${router_name}.yaml"

  getFileOverwritePermission "${output_file}"
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  "${ZITI_BIN_DIR}/ziti" create config router edge --routerName "${router_name}" --private > "${output_file}"
  echo -e "Router configuration file written to: $(BLUE "${output_file}")"
}

function createPki {
  echo "Generating PKI"

  pki_create_ca "${ZITI_CONTROLLER_ROOTCA_NAME}"
  pki_create_ca "${ZITI_EDGE_CONTROLLER_ROOTCA_NAME}"
  pki_create_ca "${ZITI_SIGNING_ROOTCA_NAME}"

  ZITI_SPURIOUS_INTERMEDIATE="${ZITI_SIGNING_INTERMEDIATE_NAME}_spurious_intermediate"
  pki_create_intermediate "${ZITI_CONTROLLER_ROOTCA_NAME}" "${ZITI_CONTROLLER_INTERMEDIATE_NAME}" 1
  pki_create_intermediate "${ZITI_EDGE_CONTROLLER_ROOTCA_NAME}" "${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}" 1
  pki_create_intermediate "${ZITI_SIGNING_ROOTCA_NAME}" "${ZITI_SPURIOUS_INTERMEDIATE}" 2
  pki_create_intermediate "${ZITI_SPURIOUS_INTERMEDIATE}" "${ZITI_SIGNING_INTERMEDIATE_NAME}" 1

  echo " "
  pki_allow_list_dns="${ZITI_CONTROLLER_HOSTNAME},localhost,${ZITI_NETWORK}"
  if [[ "${ZITI_EDGE_CONTROLLER_HOSTNAME}" != "" ]]; then pki_allow_list_dns="${pki_allow_list_dns},${ZITI_EDGE_CONTROLLER_HOSTNAME}"; fi
  if [[ "${EXTERNAL_DNS}" != "" ]]; then pki_allow_list_dns="${pki_allow_list_dns},${EXTERNAL_DNS}"; fi
  pki_allow_list_ip="127.0.0.1"
  if [[ "${ZITI_EDGE_CONTROLLER_IP_OVERRIDE}" != "" ]]; then pki_allow_list_ip="${pki_allow_list_ip},${ZITI_EDGE_CONTROLLER_IP_OVERRIDE}"; fi
  if [[ "${EXTERNAL_IP}" != "" ]]; then pki_allow_list_ip="${pki_allow_list_ip},${EXTERNAL_IP}"; fi

  pki_client_server "${pki_allow_list_dns}" "${ZITI_CONTROLLER_INTERMEDIATE_NAME}" "${pki_allow_list_ip}" "${ZITI_CONTROLLER_HOSTNAME}"
  pki_client_server "${pki_allow_list_dns}" "${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}" "${pki_allow_list_ip}" "${ZITI_EDGE_CONTROLLER_HOSTNAME}"
}



function createFabricRouterConfig {
  # Allow router name to be passed in as arg
  router_name="${1-}"
  if [[ "${router_name}" == "" ]]; then

    # If router name is not passed as arg, prompt user for input
    echo -e "$(YELLOW "createFabricRouterConfig requires a router name to be supplied") "
    default_router_name="${ZITI_EDGE_ROUTER_RAWNAME}"
    echo -en "Enter router name (${default_router_name}):"
    read -r router_name

    # Accept the default if no name provided
    if [[ "${router_name}" == "" ]]; then
      # Check for overwrite of default file
      router_name="${default_router_name}"
    fi
  fi

  checkEnvVariable ZITI_HOME
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  output_file="${ZITI_HOME}/${router_name}.yaml"

  getFileOverwritePermission "${output_file}"
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  "${ZITI_BIN_DIR}/ziti" create config router fabric --routerName "${router_name}" > "${output_file}"
  echo -e "Fabric router configuration file written to: $(BLUE "${output_file}")"
}

function createEdgeRouterWssConfig {
  # Allow router name to be passed in as arg
  router_name="${1-}"
  if [[ "${router_name}" == "" ]]; then

    # If router name is not passed as arg, prompt user for input
    echo -e "$(YELLOW "createEdgeRouterWssConfig requires a router name to be supplied") "
    default_router_name="${ZITI_EDGE_ROUTER_RAWNAME}"
    echo -en "Enter router name (${default_router_name}):"
    read -r router_name

    # Accept the default if no name provided
    if [[ "${router_name}" == "" ]]; then
      # Check for overwrite of default file
      router_name="${default_router_name}"
      getFileOverwritePermission "${ZITI_HOME-}/${router_name}.yaml"
      retVal=$?
      if [[ "${retVal}" != 0 ]]; then
        return 1
      fi
    fi
  fi

  # Make sure necessary env variables are set
  checkEnvVariable ZITI_HOME ZITI_BIN_DIR
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  output_file="${ZITI_HOME-}/${router_name}.yaml"

  getFileOverwritePermission "${output_file}"
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  "${ZITI_BIN_DIR}/ziti" create config router edge --wss --routerName "${router_name}" > "${output_file}"
  echo -e "WSS Edge router wss configuration file written to: $(BLUE "${output_file}")"
}

# shellcheck disable=SC2120
function createEdgeRouterConfig {
  # Allow router name to be passed in as arg
  router_name="${1-}"
  if [[ "${router_name}" == "" ]]; then

    # If router name is not passed as arg, prompt user for input
    echo -e "$(YELLOW "createEdgeRouterConfig requires a router name to be supplied") "
    default_router_name="${ZITI_EDGE_ROUTER_RAWNAME}"
    echo -en "Enter router name (${default_router_name}):"
    read -r router_name

    # Accept the default if no name provided
    if [[ "${router_name}" == "" ]]; then
      # Check for overwrite of default file
      router_name="${default_router_name}"
      getFileOverwritePermission "${ZITI_HOME-}/${router_name}.yaml"
      retVal=$?
      if [[ "${retVal}" != 0 ]]; then
        return 1
      fi
    fi
  fi

  # Make sure necessary env variables are set
  checkEnvVariable ZITI_HOME ZITI_BIN_DIR
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  output_file="${ZITI_HOME}/${router_name}.yaml"

  getFileOverwritePermission "${output_file}"
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  "${ZITI_BIN_DIR}/ziti" create config router edge --routerName "${router_name}" > "${output_file}"
  echo -e "edge router configuration file written to: $(BLUE "${output_file}")"
}

# shellcheck disable=SC2120
function createControllerConfig {
  # Allow controller name to be passed in as arg
  controller_name="${1-}"
  # If no controller name provided and env var is not set, prompt user for a controller name
  if [[ "${controller_name}" == "" ]] && [[ -z "${ZITI_EDGE_CONTROLLER_RAWNAME}" ]]; then
        echo -e "$(YELLOW "createControllerConfig requires a controller name to be supplied") "
        echo -en "Enter controller name: "
        read -r controller_name

        # Quit if no name is provided
        if [[ "${controller_name}" == "" ]]; then
          echo -e "$(RED "  --- Invalid controller name provided ---")"
          return 1
        fi
  # If no controller name provided and env var is set, use env var
  elif [[ "${controller_name}" == "" ]] && [[ -n "${ZITI_EDGE_CONTROLLER_RAWNAME}" ]]; then
    controller_name="${ZITI_EDGE_CONTROLLER_RAWNAME}"
  fi

  # Make sure necessary env variables are set
  checkEnvVariable ZITI_HOME
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  cat "${ZITI_CTRL_IDENTITY_SERVER_CERT}" > "${ZITI_CTRL_IDENTITY_CA}"
  cat "${ZITI_SIGNING_CERT}" >> "${ZITI_CTRL_IDENTITY_CA}"
  echo -e "wrote CA file to: $(BLUE "${ZITI_CTRL_IDENTITY_CA}")"

  output_file="${ZITI_HOME}/${controller_name}.yaml"

  getFileOverwritePermission "${output_file}"
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  "${ZITI_BIN_DIR}/ziti" create config controller > "${output_file}"

  echo -e "Controller configuration file written to: $(BLUE "${output_file}")"
}

# shellcheck disable=SC2120
function ziti_createEnvFile {
  checkEnvVariable ZITI_HOME
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    if decideToUseDefaultZitiHome; then
      ZITI_HOME="${DEFAULT_ZITI_HOME_LOCATION}"
    else
      return 1
    fi
  fi

  export ZITI_HOME="${ZITI_HOME}"
  if [[ "${ZITI_OSTYPE}" == "windows" ]]; then
    ZITI_HOME_OS_SPECIFIC="$(cygpath -m "${ZITI_HOME}")"
    export ZITI_HOME_OS_SPECIFIC
  else
    export ZITI_HOME_OS_SPECIFIC="${ZITI_HOME}"
  fi
  export ENV_FILE="${ZITI_HOME}/${ZITI_NETWORK}.env"
  export ZITI_SHARED="${ZITI_HOME}"

  checkEnvVariable ENV_FILE
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  if [[ "${network_name-}" != "" ]]; then
    export ZITI_NETWORK="${network_name}"
  fi

  if [[ "${ZITI_NETWORK-}" == "" ]]; then
    if [[ "${1-}" != "" ]]; then
      export ZITI_NETWORK="${1-}"
    fi
    if [[ "${ZITI_NETWORK-}" = "" ]]; then
      echo -e "$(YELLOW "WARN: ZITI_NETWORK HAS NOT BEEN DECLARED! USING hostname: ${DEFAULT_ZITI_NETWORK}")"
      # shellcheck disable=SC2155
      export ZITI_NETWORK="${DEFAULT_ZITI_NETWORK}"
    fi
  fi

  echo "ZITI_NETWORK set to: ${ZITI_NETWORK}"

  if [[ "${ZITI_USER-}" == "" ]]; then export ZITI_USER="admin"; fi
  if [[ "${ZITI_PWD-}" == "" ]]; then 
    ZITI_PWD="$(LC_ALL=C tr -dc _A-Z-a-z-0-9 < /dev/urandom | head -c32)"
    echo -en "Do you want to keep the generated admin password '$ZITI_PWD'? (Y/n) "
    # shellcheck disable=SC2162
    read -r pwd_reply
    if [[ -z "${pwd_reply}" || ${pwd_reply} =~ [yY] ]]; then
      echo "INFO: using ZITI_PWD=${ZITI_PWD}"
    else
      echo -en "Type the preferred admin password and press <enter>"
      read -r ZITI_PWD
    fi
  fi
  if [[ "${ZITI_DOMAIN_SUFFIX-}" == "" ]]; then export ZITI_DOMAIN_SUFFIX=""; fi
  if [[ "${ZITI_ID-}" == "" ]]; then export ZITI_ID="${ZITI_HOME}/identities.yaml"; fi

  export ZITI_PKI="${ZITI_SHARED}/pki"
  if [[ "${ZITI_OSTYPE}" == "windows" ]]; then
    ZITI_PKI_OS_SPECIFIC="$(cygpath -m "${ZITI_PKI}")"
    export ZITI_PKI_OS_SPECIFIC
  else
    export ZITI_PKI_OS_SPECIFIC="${ZITI_PKI}"
  fi
  if [[ "${ZITI_EDGE_CONTROLLER_PORT-}" == "" ]]; then export ZITI_EDGE_CONTROLLER_PORT="1280"; fi

  if [[ "${ZITI_CONTROLLER_HOSTNAME-}" == "" ]]; then export ZITI_CONTROLLER_HOSTNAME="${ZITI_CONTROLLER_RAWNAME}${ZITI_DOMAIN_SUFFIX}"; fi
  if [[ "${ZITI_CTRL_ADVERTISED_ADDRESS-}" == "" ]]; then export ZITI_CTRL_ADVERTISED_ADDRESS="${ZITI_CONTROLLER_HOSTNAME}"; fi
  if [[ "${ZITI_EDGE_CONTROLLER_HOSTNAME-}" == "" ]]; then export ZITI_EDGE_CONTROLLER_HOSTNAME="${ZITI_EDGE_CONTROLLER_RAWNAME}${ZITI_DOMAIN_SUFFIX}"; fi
  if [[ "${ZITI_ZAC_HOSTNAME-}" == "" ]]; then export ZITI_ZAC_HOSTNAME="${ZITI_ZAC_RAWNAME}${ZITI_DOMAIN_SUFFIX}"; fi
  if [[ "${ZITI_EDGE_CTRL_ADVERTISED-}" == "" ]]; then export ZITI_EDGE_CTRL_ADVERTISED="${ZITI_EDGE_CONTROLLER_HOSTNAME}:${ZITI_EDGE_CONTROLLER_PORT}"; fi

  export ZITI_SIGNING_CERT_NAME="${ZITI_NETWORK}-signing"

  export ZITI_CONTROLLER_ROOTCA_NAME="${ZITI_CONTROLLER_HOSTNAME}-root-ca"
  export ZITI_CONTROLLER_INTERMEDIATE_NAME="${ZITI_CONTROLLER_HOSTNAME}-intermediate"

  export ZITI_EDGE_CONTROLLER_ROOTCA_NAME="${ZITI_EDGE_CONTROLLER_HOSTNAME}-root-ca"
  export ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME="${ZITI_EDGE_CONTROLLER_HOSTNAME}-intermediate"

  export ZITI_SIGNING_ROOTCA_NAME="${ZITI_SIGNING_CERT_NAME}-root-ca"
  export ZITI_SIGNING_INTERMEDIATE_NAME="${ZITI_SIGNING_CERT_NAME}-intermediate"

  if [[ "${ZITI_CTRL_MGMT_PORT-}" == "" ]]; then export ZITI_CTRL_MGMT_PORT="10000"; fi
  if [[ "${ZITI_CTRL_MGMT_HOST_PORT-}" == "" ]]; then export ZITI_CTRL_MGMT_HOST_PORT="${ZITI_CONTROLLER_HOSTNAME}:${ZITI_CTRL_MGMT_PORT}"; fi
  if [[ "${ZITI_CTRL_IDENTITY_CERT-}" == "" ]]; then export ZITI_CTRL_IDENTITY_CERT="${ZITI_PKI_OS_SPECIFIC}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_CONTROLLER_HOSTNAME}-client.cert"; fi
  if [[ "${ZITI_CTRL_IDENTITY_SERVER_CERT-}" == "" ]]; then export ZITI_CTRL_IDENTITY_SERVER_CERT="${ZITI_PKI_OS_SPECIFIC}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_CONTROLLER_HOSTNAME}-server.chain.pem"; fi
  if [[ "${ZITI_CTRL_IDENTITY_KEY-}" == "" ]]; then export ZITI_CTRL_IDENTITY_KEY="${ZITI_PKI_OS_SPECIFIC}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/keys/${ZITI_CONTROLLER_HOSTNAME}-server.key"; fi
  if [[ "${ZITI_CTRL_IDENTITY_CA-}" == "" ]]; then export ZITI_CTRL_IDENTITY_CA="${ZITI_PKI_OS_SPECIFIC}/cas.pem"; fi
  if [[ "${ZITI_EDGE_CTRL_ADVERTISED_HOST_PORT}" == "" ]]; then export ZITI_EDGE_CTRL_ADVERTISED_HOST_PORT="${ZITI_EDGE_CONTROLLER_HOSTNAME}:${ZITI_EDGE_CONTROLLER_PORT}"; fi
  if [[ "${ZITI_EDGE_CTRL_IDENTITY_CERT}" == "" ]]; then export ZITI_EDGE_CTRL_IDENTITY_CERT="${ZITI_PKI_OS_SPECIFIC}/${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_EDGE_CONTROLLER_HOSTNAME}-client.cert"; fi
  if [[ "${ZITI_EDGE_CTRL_IDENTITY_SERVER_CERT}" == "" ]]; then export ZITI_EDGE_CTRL_IDENTITY_SERVER_CERT="${ZITI_PKI_OS_SPECIFIC}/${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_EDGE_CONTROLLER_HOSTNAME}-server.chain.pem"; fi
  if [[ "${ZITI_EDGE_CTRL_IDENTITY_KEY}" == "" ]]; then export ZITI_EDGE_CTRL_IDENTITY_KEY="${ZITI_PKI_OS_SPECIFIC}/${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}/keys/${ZITI_EDGE_CONTROLLER_HOSTNAME}-server.key"; fi
  if [[ "${ZITI_EDGE_CTRL_IDENTITY_CA}" == "" ]]; then export ZITI_EDGE_CTRL_IDENTITY_CA="${ZITI_PKI_OS_SPECIFIC}/${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}.cert"; fi

  if [[ "${ZITI_SIGNING_CERT}" == "" ]]; then export ZITI_SIGNING_CERT="${ZITI_PKI_OS_SPECIFIC}/${ZITI_SIGNING_INTERMEDIATE_NAME}/certs/${ZITI_SIGNING_INTERMEDIATE_NAME}.cert"; fi
  if [[ "${ZITI_SIGNING_KEY}" == "" ]]; then export ZITI_SIGNING_KEY="${ZITI_PKI_OS_SPECIFIC}/${ZITI_SIGNING_INTERMEDIATE_NAME}/keys/${ZITI_SIGNING_INTERMEDIATE_NAME}.key"; fi

  if [[ "${ZITI_EDGE_ROUTER_RAWNAME-}" == "" ]]; then ZITI_EDGE_ROUTER_RAWNAME="${ZITI_NETWORK}-edge-router"; fi

  mkdir -p "${ZITI_BIN_ROOT}"
  mkdir -p "${ZITI_HOME}/db"
  mkdir -p "${ZITI_PKI}"

  echo "" > "${ENV_FILE}"
  for zEnvVar in $(set | grep -e "^ZITI_" | sort); do envvar="$(echo "${zEnvVar}" | cut -d '=' -f1)"; envval="$(echo "${zEnvVar}" | cut -d '=' -f2-100)"; echo "export ${envvar}=\"${envval}\"" >> "${ENV_FILE}"; done

  export PFXLOG_NO_JSON=true
  # shellcheck disable=SC2129
  echo "export PFXLOG_NO_JSON=true" >> "${ENV_FILE}"

  echo "alias zec='ziti edge'" >> "${ENV_FILE}"
  echo "alias zlogin='ziti edge login \"\${ZITI_EDGE_CTRL_ADVERTISED}\" -u \"\${ZITI_USER-}\" -p \"\${ZITI_PWD}\" -c \"\${ZITI_PKI}/\${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}/certs/\${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}.cert\"'" >> "${ENV_FILE}"
  echo "alias zitiLogin='ziti edge login \"\${ZITI_EDGE_CTRL_ADVERTISED}\" -u \"\${ZITI_USER-}\" -p \"\${ZITI_PWD}\" -c \"\${ZITI_PKI}/\${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}/certs/\${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}.cert\"'" >> "${ENV_FILE}"
  echo "alias psz='ps -ef | grep ziti'" >> "${ENV_FILE}"

  #when sourcing the emitted file add the bin folder to the path
  tee -a "${ENV_FILE}" > /dev/null <<'heredoc'
echo " "
if [[ ! "$(echo "$PATH"|grep -q "${ZITI_BIN_DIR}" && echo "yes")" == "yes" ]]; then
  echo "adding ${ZITI_BIN_DIR} to the path"
  export PATH=$PATH:"${ZITI_BIN_DIR}"
else
echo    "                  ziti binaries are located at: ${ZITI_BIN_DIR}"
echo -e 'add this to your path if you want by executing: export PATH=$PATH:'"${ZITI_BIN_DIR}"
echo " "
fi
heredoc

echo -e "env file written to: $(BLUE "${ENV_FILE}")"
}

function waitForController {
  #devnull="/dev/null"
  #if [[ "${ZITI_OSTYPE}" == "windows" ]]; then
  #  devnull="nul"
  #fi
  # shellcheck disable=SC2091
  #until $(curl -o /dev/null -sk /dev/null --fail "https://${ZITI_EDGE_CTRL_ADVERTISED}"); do
  #    echo "waiting for https://${ZITI_EDGE_CTRL_ADVERTISED}"
  #    sleep 2
  #done
  while [[ "$(curl -w "%{http_code}" -m 1 -s -k -o /dev/null https://"${ZITI_EDGE_CTRL_ADVERTISED}"/version)" != "200" ]]; do
    echo "waiting for https://${ZITI_EDGE_CTRL_ADVERTISED}"
    sleep 3
  done
}

function createControllerSystemdFile {
  # Allow controller name to be passed in as arg
  controller_name="${1-}"
  # If no controller name provided and env var is not set, prompt user for a controller name
  if [[ "${controller_name}" == "" ]] && [[ -z "${ZITI_EDGE_CONTROLLER_RAWNAME}" ]]; then
        echo -e "$(YELLOW "createControllerSystemdFile requires a controller name to be supplied") "
        echo -en "Enter controller name:"
        read -r controller_name

        # Quit if no name is provided
        if [[ "${controller_name}" == "" ]]; then
          echo -e "$(RED "  --- Invalid controller name provided ---")"
          return 1
        fi
  # If no controller name provided and env var is set, use env var
  elif [[ "${controller_name}" == "" ]] && [[ -n "${ZITI_EDGE_CONTROLLER_RAWNAME}" ]]; then
    controller_name="${ZITI_EDGE_CONTROLLER_RAWNAME}"
  fi

  # Make sure necessary env variables are set
  checkEnvVariable ZITI_HOME ZITI_BIN_DIR
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  output_file="${ZITI_HOME}/${controller_name}.service"

  getFileOverwritePermission "${output_file}"
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
ExecStart="${ZITI_BIN_DIR}/ziti-controller" run "${ZITI_HOME}/${controller_name}.yaml"
Restart=always
RestartSec=2
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target

HeredocForSystemd
  echo -e "Controller systemd file written to: $(BLUE "${output_file}")"
}

function createRouterSystemdFile {
  # Allow router name to be passed in as arg
  router_name="${1-}"
  if [[ "${router_name}" == "" ]]; then

    # If router name is not passed as arg, prompt user for input
    echo -e "$(YELLOW "createRouterSystemdFile requires a router name to be supplied") "
    default_router_name="${ZITI_EDGE_ROUTER_RAWNAME}"
    echo -en "Enter router name (${default_router_name}):"
    read -r router_name

    # Accept the default if no name provided
    if [[ "${router_name}" == "" ]]; then
      # Check for overwrite of default file
      router_name="${default_router_name}"
      getFileOverwritePermission "${ZITI_HOME-}/${router_name}.service"
      retVal=$?
      if [[ "${retVal}" != 0 ]]; then
        return 1
      fi
    fi
  fi

  checkEnvVariable ZITI_HOME ZITI_BIN_DIR
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  output_file="${ZITI_HOME}/${router_name}.service"

  getFileOverwritePermission "${output_file}"
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
ExecStart="${ZITI_BIN_DIR}/ziti-router" run "${ZITI_HOME}/${router_name}.yaml"
Restart=always
RestartSec=2
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target

HeredocForSystemd
  echo -e "Router systemd file written to: $(BLUE "${output_file}")"
}

function createControllerLaunchdFile {
  # Allow controller name to be passed in as arg
  controller_name="${1-}"
  # If no controller name provided and env var is not set, prompt user for a controller name
  if [[ "${controller_name}" == "" ]] && [[ -z "${ZITI_EDGE_CONTROLLER_RAWNAME}" ]]; then
        echo -e "$(YELLOW "createControllerLaunchdFile requires a controller name to be supplied") "
        echo -en "Enter controller name: "
        read -r controller_name

        # Quit if no name is provided
        if [[ "${controller_name}" == "" ]]; then
          echo -e "$(RED "  --- Invalid controller name provided ---")"
          return 1
        fi
  # If no controller name provided and env var is set, use env var
  elif [[ "${controller_name}" == "" ]] && [[ -n "${ZITI_EDGE_CONTROLLER_RAWNAME}" ]]; then
    controller_name="${ZITI_EDGE_CONTROLLER_RAWNAME}"
  fi

  # Make sure necessary env variables are set
  checkEnvVariable ZITI_HOME ZITI_BIN_DIR
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  output_file="${ZITI_HOME}/${controller_name}.plist"

  getFileOverwritePermission "${output_file}"
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

cat > "${output_file}" <<HeredocForLaunchd
<?xml version="1.0" encoding="UTF-8"?>
  <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
  <plist version="1.0">
    <dict>
      <key>Label</key>
      <string>ziti-controller-${controller_name}</string>
      <key>ProgramArguments</key>
      <array>
        <string>$ZITI_BIN_DIR/ziti-controller</string>
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
  # Allow router name to be passed in as arg
  router_name="${1-}"
  if [[ "${router_name}" == "" ]]; then

    # If router name is not passed as arg, prompt user for input
    echo -e "$(YELLOW "createRouterLaunchdFile requires a router name to be supplied") "
    default_router_name="${ZITI_EDGE_ROUTER_RAWNAME}"
    echo -en "Enter router name (${default_router_name}):"
    read -r router_name

    # Accept the default if no name provided
    if [[ "${router_name}" == "" ]]; then
      # Check for overwrite of default file
      router_name="${default_router_name}"
      getFileOverwritePermission "${ZITI_HOME-}/${router_name}.plist"
      retVal=$?
      if [[ "${retVal}" != 0 ]]; then
        return 1
      fi
    fi
  fi

  # Make sure necessary env variables are set
  checkEnvVariable ZITI_HOME ZITI_BIN_DIR
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  output_file="${ZITI_HOME-}/${router_name}.plist"

  getFileOverwritePermission "${output_file}"
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

cat > "${output_file}" <<HeredocForLaunchd
<?xml version="1.0" encoding="UTF-8"?>
  <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
  <plist version="1.0">
    <dict>
      <key>Label</key>
      <string>$router_name</string>
      <key>ProgramArguments</key>
      <array>
        <string>$ZITI_BIN_DIR/ziti-router</string>
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

  checkEnvVariable ZITI_HOME
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  output_file="${ZITI_HOME}/ziti-console.service"

  getFileOverwritePermission "${output_file}"
  retVal=$?
  if [[ "${retVal}" != 0 ]]; then
    return 1
  fi

  if which node >/dev/null; then
    # store the absolute path to the node executable because it's required by systemd on Amazon Linux, at least
    NODE_BIN=$(readlink -f "$(which node)")
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
ExecStart=${NODE_BIN} "${ZITI_HOME}/ziti-console/server.js"
Restart=always
RestartSec=2
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target

HeredocForSystemd
  echo -e "ziti-console systemd file written to: $(BLUE "${output_file}")"
}

function setOs {
  if [ -n "${ZITI_OSTYPE}" ]; then return; fi
  if [[ "$OSTYPE" == "linux-gnu"* ]]; then
          export ZITI_OSTYPE="linux"
  elif [[ "$OSTYPE" == "darwin"* ]]; then
          export ZITI_OSTYPE="darwin"
  elif [[ "$OSTYPE" == "cygwin" ]]; then
          export ZITI_OSTYPE="windows"
          #echo -e "  * ERROR: $(RED "\$OSTYPE [$OSTYPE] is not supported at this time") "
          #return 1
  elif [[ "$OSTYPE" == "msys" ]]; then
          export ZITI_OSTYPE="windows"
          #echo -e "  * ERROR: $(RED "\$OSTYPE [$OSTYPE] is not supported at this time") "
          #return 1
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

function checkEnvVariable() {

  for arg
  do
    # Parameter expansion is different between shells
    if [[ -n "$ZSH_VERSION" ]]; then
      # shellcheck disable=SC2296
      if [[ -z "${(P)arg}" ]]; then
        echo -e "  * ERROR: $(RED "${arg} is not set") "
        return 1
      fi
    elif [[ -n "$BASH_VERSION" ]]; then
      if [[ -z "${!arg}" ]]; then
        echo -e "  * ERROR: $(RED "${arg} is not set") "
        return 1
      fi
    else
      echo -e " * $(RED "Unsupported shell, supply a PR or log an issue on https://github.com/openziti/ziti") "
      return 1
    fi
  done
  return 0
}

function getFileOverwritePermission() {
  file_path="${1-}"

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

set +uo pipefail
