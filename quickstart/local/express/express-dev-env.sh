#!/bin/bash

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

ZITI_QUICKSTART_ROOT="${HOME}/.ziti/quickstart"
ZITI_SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

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
  echo -e "$(BLUE "${ZITI_QUICKSTART_ROOT}")"
  echo " "
}

function getLatestZiti {
  if [[ "${ZITI_HOME}" == "" ]]; then
    echo "ERROR: ZITI_HOME is not set!"
    exit 1
  fi

  zitilatest=$(curl -s https://api.github.com/repos/openziti/ziti/releases/latest)
  zitiversion=$(echo ${zitilatest} | jq -r '.tag_name')
  echo "Latest ziti is: ${zitiversion}"
  zititgz=$(echo ${zitilatest} | jq -r '.assets[] | select(.name | startswith("ziti-linux-amd")) | .name')

  if ! test -f "${ZITI_HOME}/${zititgz}"; then
    echo -e 'Downloading '"$(BLUE "${zititgz}")"' into '"$(BLUE "${ZITI_HOME}")"
    zitidl="https://github.com/openziti/ziti/releases/download/${zitiversion}/${zititgz}"

    wget -q "${zitidl}" -O "${ZITI_HOME}/${zititgz}"
  fi

  mkdir -p "${ZITI_BIN_DIR}"
  echo -e 'UNZIPPING '"$(BLUE "${ZITI_HOME}/${zititgz}")"' into: '"$(GREEN ${ZITI_BIN_DIR})"
  tar -xf "${ZITI_HOME}/${zititgz}" --directory "${ZITI_BIN_DIR}"

  echo "Adding ${ZITI_BIN} to the path if necessary:"
  if [[ "$(echo "$PATH"|grep -q "${ZITI_BIN}" && echo "yes")" == "yes" ]]; then
    echo -e "$(GREEN "${ZITI_BIN}") is already on the path"
  else
    echo -e "adding $(RED "${ZITI_BIN}") to the path"
    export PATH=$PATH:"${ZITI_BIN}"
  fi

}

function generatePki {
  PKI_DIR="${ZITI_PKI}/${ZITI_CONTROLLER_ROOTCA_NAME}"
  #if [ -d "${PKI_DIR}" ]
  #then
  #    echo -e "Reusing existing PKI from: $(BLUE ${PKI_DIR})"
  #else
      echo "Generating PKI"
      . "${ZITI_SCRIPT_DIR}/../create-pki.sh"
  #fi
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
      exit 1
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

function generateEnvFile {
  if [[ "$1" == "" ]]; then
    echo " "
    echo "Creating a controller is effectively creating a network. The name of the network will be used when writing"
    echo "configuration files locally. Choose the name of your network now. The format of the network name should resemble"
    echo -n "what a hostname looks like. A good choice is to actually use your system's hostname: "
    echo -e "$(BLUE $(hostname))"
    echo " "
    read -p "$(echo -ne "Network Name [$(BLUE $(hostname))]: ")" ZITI_NETWORK
    echo " "
    if checkControllerName; then
      : #clear to continue
      if [[ "ZITI_NETWORK" == "" ]]; then
        ZITI_NETWORK="$(hostname)"
      fi
      echo "name: ${ZITI_NETWORK}"
    else
      echo " "
      echo "nah bro"
      return 1
    fi
    echo " "
  else
    ZITI_NETWORK="$1"
  fi

  echo -e "Generating new network with name: $(BLUE "${ZITI_NETWORK}")"

  export ZITI_CONTROLLER_RAWNAME="${ZITI_NETWORK}"
  export ZITI_EDGE_CONTROLLER_RAWNAME="${ZITI_NETWORK}"
  export ZITI_ZAC_RAWNAME="${ZITI_NETWORK}"
  export ZITI_EDGE_ROUTER_RAWNAME="${ZITI_NETWORK}"
  export ZITI_EDGE_WSS_ROUTER_RAWNAME="${ZITI_NETWORK}"
  export ZITI_ROUTER_BR_RAWNAME="${ZITI_NETWORK}"
  export ZITI_ROUTER_BLUE_RAWNAME="${ZITI_NETWORK}"
  export ZITI_ROUTER_RED_RAWNAME="${ZITI_NETWORK}"

  export ZITI_HOME=${HOME}/.ziti/quickstart/${ZITI_NETWORK}
  export ENV_FILE="${ZITI_HOME}/${ZITI_NETWORK}.env"
  "${ZITI_SCRIPT_DIR}/../../docker/image/env.sh"
  echo -e "environment file created and source from: $(BLUE ${ENV_FILE})"
  source "${ENV_FILE}"
}

function expressConfiguration {
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

  if [[ "$1" == "" ]]; then
    nw="$(hostname)"
  else
    nw="$1"
  fi
  generateEnvFile "${nw}"
  #checkHostsFile
  getLatestZiti
  generatePki
  generateControllerConfig
  generateEdgeRouterConfig
  initializeController
  startZitiController
  echo "starting the ziti controller to enroll the edge router"
  sleep 2
  zitiLogin

  echo -e "----------  Creating an edge router policy allowing all identities to connect to routers with a $(GREEN "#public") attribute"
  unused=$(ziti edge controller create edge-router-policy allEdgeRouters --edge-router-roles '#public' --identity-roles '#all' )

  echo -e "----------  Creating a service edge router policy allowing all services to use $(GREEN "#public") edge routers"
  unused=$(ziti edge controller create service-edge-router-policy allSvcPublicRouters --edge-router-roles '#public' --service-roles '#all')

  echo "----------  Creating edge-router ${ZITI_EDGE_ROUTER_HOSTNAME}...."
  unused=$(ziti edge controller create edge-router "${ZITI_EDGE_ROUTER_HOSTNAME}" -o "${ZITI_HOME}/${ZITI_EDGE_ROUTER_HOSTNAME}.jwt" -t)
  sleep 1
  echo "---------- Enrolling edge-router ${ZITI_EDGE_ROUTER_HOSTNAME}...."
  unused=$(ziti-router enroll "${ZITI_HOME}/${ZITI_EDGE_ROUTER_HOSTNAME}.yaml" --jwt "${ZITI_HOME}/${ZITI_EDGE_ROUTER_HOSTNAME}.jwt" &> "${ZITI_EDGE_ROUTER_HOSTNAME}.enrollment.log")
  echo ""
  sleep 1
  unused=$(ziti-router run "${ZITI_HOME}/${ZITI_EDGE_ROUTER_HOSTNAME}.yaml" > "${ZITI_HOME}/ziti-${ZITI_EDGE_ROUTER_HOSTNAME}.log" 2>&1 &)

}
function zitiLogin {
  unused=$(ziti edge controller login "${ZITI_EDGE_CONTROLLER_API}" -u "${ZITI_USER}" -p "${ZITI_PWD}" -c "${ZITI_PKI}/${ZITI_EDGE_CONTROLLER_ROOTCA_NAME}/certs/${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}.cert")
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
  "${ZITI_SCRIPT_DIR}/../create-controller-config.sh"
}
function generateEdgeRouterConfig {
  "${ZITI_SCRIPT_DIR}/../create-edge-router-config.sh"
}
function initializeController {
  ziti-controller edge init "${ZITI_HOME}/controller.yaml" -u "${ZITI_USER}" -p "${ZITI_PWD}" &> "${ZITI_HOME}/controller-init.log"
  echo -e "ziti-controller initialized. see $(BLUE ${ZITI_HOME}/controller-init.log) for details"
}
function startZitiController {
  unused=$(ziti-controller run "${ZITI_HOME}/controller.yaml" > "${ZITI_HOME}/ziti-controller.log" 2>&1 &)
  echo -e "ziti-controller started. log located at: $(BLUE ${ZITI_HOME}/ziti-controller.log)"
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
    echo -e $(YELLOW "Ziti is generally used to create an overlay network. Generally speaking this will involve more than one host")
    echo -e $(YELLOW "Since this is a script geared towards setting up a very minimal development environment it needs to make some")
    echo -e $(YELLOW "assumptions. One of these assumptions is that the three specific entries are entered onto your hosts file.")
    echo -e $(YELLOW "One or more of these are missing:")
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
    exit 1
  fi
}

function decideOperation {
  if [[ "$1" != "" ]]; then
    yn=$1
  fi
  while true
  do
    if [[ "${yn}" == "" ]]; then
      echo "What sort of operation are you looking to perform?"
      echo "  1.) Express configuration - a simple overlay will be spawned containing one controller and two edge routers"
      echo "  2.) Create Controller configuration - answer a few questions and a controller config will be emitted"
      echo "  3.) Create Edge Router configuration - answer a few questions and an edge router config will be emitted"
      echo "  4.) Start a network with the provided name"
      echo " "
      read -p "Select an action: " yn
    fi
    case $yn in
        [1]* )
            expressConfiguration "$2"
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

#greet the user with the banner and quick blurb about what to expect
issueGreeting

#make sure the user has all the necessary commands to be successful
checkPrereqs

#prompt the user for input and do what they want/need
decideOperation 1 "$1"

