#!/bin/bash

export ZITI_QUICKSTART_SCRIPT_ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
export ZITI_QUICKSTART_ENVROOT="${HOME}/.ziti/quickstart"

ASCI_WHITE='\033[01;37m'
ASCI_RESTORE='\033[0m'
ASCI_RED='\033[00;31m'
ASCI_GREEN='\033[00;32m'
ASCI_YELLOW='\033[00;33m'
ASCI_BLUE='\033[00;34m'
ASCI_MAGENTA='\033[00;35m'
ASCI_PURPLE='\033[00;35m'
ASCI_CYAN='\033[00;36m'
ASCI_LIGHTGRAY='\033[00;37m'
ASCI_LRED='\033[01;31m'
ASCI_LGREEN='\033[01;32m'
ASCI_LYELLOW='\033[01;33m'
ASCI_LBLUE='\033[01;34m'
ASCI_LMAGENTA='\033[01;35m'
ASCI_LPURPLE='\033[01;35m'
ASCI_LCYAN='\033[01;36m'

function WHITE {
  echo "${ASCI_WHITE}$1${ASCI_RESTORE}"
}
function RED {
  echo "${ASCI_RED}$1${ASCI_RESTORE}"
}
function GREEN {
  echo "${ASCI_GREEN}$1${ASCI_RESTORE}"
}
function YELLOW {
  echo "${ASCI_YELLOW}$1${ASCI_RESTORE}"
}
function BLUE {
  echo "${ASCI_BLUE}$1${ASCI_RESTORE}"
}

function zitiLogin {
  unused=$("${ZITI_BIN_DIR}/ziti" edge login "${ZITI_EDGE_CONTROLLER_API}" -u "${ZITI_USER}" -p "${ZITI_PWD}" -c "${ZITI_PKI}/${ZITI_EDGE_CONTROLLER_ROOTCA_NAME}/certs/${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}.cert")
}
function reinitializeZitiController {
  cleanZitiController
}
function cleanZitiController {
  rm -rf "${ZITI_HOME}/db"
  mkdir "${ZITI_HOME}/db"
  initializeController
}
function generateControllerConfig {
  "${ZITI_QUICKSTART_SCRIPT_ROOT}/docker/image/create-controller-config.sh"
}
function generateEdgeRouterConfig {
  echo "RUNNING: ${ZITI_QUICKSTART_SCRIPT_ROOT}/docker/image/create-edge-router-config.sh"
  "${ZITI_QUICKSTART_SCRIPT_ROOT}/docker/image/create-edge-router-config.sh"
}
function initializeController {
  "${ZITI_BIN_DIR}/ziti-controller" edge init "${ZITI_HOME}/controller.yaml" -u "${ZITI_USER}" -p "${ZITI_PWD}" &> "${ZITI_HOME}/controller-init.log"
  echo -e "ziti-controller initialized. see $(BLUE ${ZITI_HOME}/controller-init.log) for details"
}
function startZitiController {
  # shellcheck disable=SC2034
  unused=$("${ZITI_BIN_DIR}/ziti-controller" run "${ZITI_HOME}/controller.yaml" > "${ZITI_HOME}/ziti-edge-controller.log" 2>&1 &)
  echo -e "ziti-controller started. log located at: $(BLUE ${ZITI_HOME}/ziti-edge-controller.log)"
}
function stopZitiController {
  killall ziti-controller
}
function checkHostsFile {
  ctrlexists=$(grep -c ${ZITI_CONTROLLER_HOSTNAME} /etc/hosts)
  edgectrlexists=$(grep -c ${ZITI_EDGE_CONTROLLER_HOSTNAME} /etc/hosts)
  erexists=$(grep -c ${ZITI_EDGE_ROUTER_HOSTNAME} /etc/hosts)

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
    return
  fi
}

function getLatestZitiVersion {
  if [[ "${ZITI_BINARIES_VERSION}" == "" ]]; then
    zitilatest=$(curl -s https://api.github.com/repos/openziti/ziti/releases/latest)
    echo ${zitilatest} > /mnt/v/temp/a.txt
    export ZITI_BINARIES_TARFILE=$(echo "${zitilatest}" | jq -r '.assets[] | select(.name | startswith("ziti-linux-amd")) | .name')
    # shellcheck disable=SC2155
    export ZITI_BINARIES_VERSION=$(echo "${zitilatest}" | jq -r '.tag_name')
  fi
  #echo -e 'Ziti tar.gz: '"$(BLUE "${ZITI_BINARIES_TARFILE}")"
  #echo -e 'Latest ziti is: '"$(BLUE "${ZITI_BINARIES_VERSION}")"
}

function getLatestZiti {
  if [[ "${ZITI_HOME}" == "" ]]; then
    echo "ERROR: ZITI_HOME is not set!"
    return
  fi
  if [[ "${ZITI_BIN_ROOT}" == "" ]]; then
    export ZITI_BIN_ROOT="${ZITI_HOME}/ziti-bin"
  fi
  mkdir -p "${ZITI_BIN_ROOT}"
  getLatestZitiVersion
  export ZITI_BIN_DIR="${ZITI_BIN_ROOT}/ziti-${ZITI_BINARIES_VERSION}"

  ZITI_BINARIES_TARFILE_ABSPATH="${ZITI_HOME}/ziti-bin/${ZITI_BINARIES_TARFILE}"
  if ! test -f "${ZITI_BINARIES_TARFILE_ABSPATH}"; then
    echo -e 'Downloading '"$(BLUE "${ZITI_BINARIES_TARFILE}")"' to '"$(BLUE "${ZITI_BINARIES_TARFILE_ABSPATH}")"
    zitidl="https://github.com/openziti/ziti/releases/download/${ZITI_BINARIES_VERSION}/${ZITI_BINARIES_TARFILE}"
    wget -q "${zitidl}" -O "${ZITI_BINARIES_TARFILE_ABSPATH}"
  else
    echo -e "$(YELLOW 'Already Downloaded ')""$(BLUE "${ZITI_BINARIES_TARFILE}")"' at: '"${ZITI_BINARIES_TARFILE_ABSPATH}"
  fi

  echo -e 'UNZIPPING '"$(BLUE "${ZITI_BINARIES_TARFILE_ABSPATH}")"' into: '"$(GREEN ${ZITI_BIN_DIR})"
  rm -rf "${ZITI_BIN_ROOT}/ziti-${ZITI_BINARIES_VERSION}"
  tar -xf "${ZITI_BINARIES_TARFILE_ABSPATH}" --directory "${ZITI_BIN_ROOT}"
  mv "${ZITI_BIN_ROOT}/ziti" "${ZITI_BIN_DIR}"

  if [[ "$1" == "yes" ]]; then
    echo "Adding ${ZITI_BIN_DIR} to the path if necessary:"
    if [[ "$(echo "$PATH"|grep -q "${ZITI_BIN_DIR}" && echo "yes")" == "yes" ]]; then
      echo -e "$(GREEN "${ZITI_BIN_DIR}") is already on the path"
    else
      echo -e "adding $(RED "${ZITI_BIN_DIR}") to the path"
      export PATH=$PATH:"${ZITI_BIN_DIR}"
    fi
  fi
}

function generatePki {
  echo "Generating PKI"
  "${ZITI_QUICKSTART_SCRIPT_ROOT}/docker/image/create-pki.sh"
}

function checkPrereqs {
  commands_to_test=(curl jq wget)

  # verify all the commands required in the automation exist before trying to run the full suite
  for cmd in "${commands_to_test[@]}"
  do
      # checking all commands are on the path before continuing...
      if ! [[ -x "$(command -v ${cmd})" ]]; then
          missing_requirements="${missing_requirements}    * ${cmd}\n"
      fi
  done
  # are requirements ? if yes, stop here and help 'em out
  if ! [[ "" = "${missing_requirements}" ]]; then
      echo " "
      echo "You're missing one or more commands that are used in this script."
      echo "Please ensure the commands listed are on the path and then try again."
      printf "\n${missing_requirements}"
      echo " "
      echo " "
      return
  fi
  echo "Let's get stated creating your local development network!"
  echo ""
  echo ""
}

function checkControllerName {
  if [[ ${ZITI_EDGE_CONTROLLER_HOSTNAME} == *['!'@#\$%^\&*()_+]* ]]; then
    echo -e "$(RED "  - The provided Network name contains an invalid character: '!'@#\$%^\&*()_+")"
    return 1
  fi
  return 0
}

function unsetZitiEnv {
  for zEnvVar in $(set -o posix ; set | grep -e "^ZITI_" | sort); do envvar="$(echo "${zEnvVar}" | cut -d '=' -f1)"; echo unsetting "[${envvar}]${zEnvVar}"; unset "${envvar}"; done
}