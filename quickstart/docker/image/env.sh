#!/bin/bash
if [[ "${ZITI_HOME}" == "" ]]; then
  echo "ERROR: ZITI_HOME HAS NOT BEEN DECLARED!"
  exit 1
fi
if [[ "${network_name}" != "" ]]; then
  export ZITI_NETWORK=${network_name}
fi
if [[ "${1}" != "" ]]; then
  export ZITI_NETWORK=${1}
fi
if [[ "${ZITI_NETWORK}" = "" ]]; then
  export ZITI_NETWORK=$(hostname)
fi

export ENV_FILE="${ZITI_HOME}/${ZITI_NETWORK}.env"
export ZITI_USER="admin"
export ZITI_PWD="admin"
export ZITI_DOMAIN_SUFFIX=".openziti.org"
export ZITI_DOMAIN_SUFFIX=""
export ZITI_ID="${ZITI_HOME}/identities.yml"
export ZITI_FAB_MGMT_PORT="10000"
export ZITI_FAB_CTRL_PORT="6262"

if [[ "${ZITI_CONTROLLER_RAWNAME}" == "" ]]; then export ZITI_CONTROLLER_RAWNAME="${ZITI_NETWORK}-controller"; fi
if [[ "${ZITI_EDGE_CONTROLLER_RAWNAME}" == "" ]]; then export ZITI_EDGE_CONTROLLER_RAWNAME="${ZITI_NETWORK}-edge-controller"; fi
if [[ "${ZITI_EDGE_ROUTER_RAWNAME}" == "" ]]; then export ZITI_EDGE_ROUTER_RAWNAME="${ZITI_NETWORK}-edge-router"; fi
if [[ "${ZITI_EDGE_ROUTER_RAWNAME_WSS}" == "" ]]; then export ZITI_EDGE_ROUTER_RAWNAME_WSS="${ZITI_NETWORK}-wss-edge-router"; fi
if [[ "${ZITI_ROUTER_BR_RAWNAME}" == "" ]]; then export ZITI_ROUTER_BR_RAWNAME="${ZITI_NETWORK}-edge-router-br"; fi
if [[ "${ZITI_ROUTER_BLUE_RAWNAME}" == "" ]]; then export ZITI_ROUTER_BLUE_RAWNAME="${ZITI_NETWORK}-edge-router-blue"; fi
if [[ "${ZITI_ROUTER_RED_RAWNAME}" == "" ]]; then export ZITI_ROUTER_RED_RAWNAME="${ZITI_NETWORK}-edge-router-red"; fi
if [[ "${ZITI_ZAC_RAWNAME}" == "" ]]; then export ZITI_ZAC_RAWNAME="${ZITI_NETWORK}-zac"; fi

export ZITI_PKI="${ZITI_HOME}/pki"
export ZITI_EDGE_CONTROLLER_PORT="1280"

export ZITI_CONTROLLER_HOSTNAME="${ZITI_CONTROLLER_RAWNAME}${ZITI_DOMAIN_SUFFIX}"
export ZITI_EDGE_CONTROLLER_HOSTNAME="${ZITI_EDGE_CONTROLLER_RAWNAME}${ZITI_DOMAIN_SUFFIX}"
export ZITI_EDGE_ROUTER_HOSTNAME="${ZITI_EDGE_ROUTER_RAWNAME}${ZITI_DOMAIN_SUFFIX}"
export ZITI_EDGE_ROUTER_RAWNAME_WSS="${ZITI_EDGE_ROUTER_RAWNAME_WSS}${ZITI_DOMAIN_SUFFIX}"
export ZITI_ROUTER_BR_HOSTNAME="${ZITI_ROUTER_BR_RAWNAME}${ZITI_DOMAIN_SUFFIX}"
export ZITI_ROUTER_BLUE_HOSTNAME="${ZITI_ROUTER_BLUE_RAWNAME}${ZITI_DOMAIN_SUFFIX}"
export ZITI_ROUTER_RED_HOSTNAME="${ZITI_ROUTER_RED_RAWNAME}${ZITI_DOMAIN_SUFFIX}"
export ZITI_ZAC_HOSTNAME="${ZITI_ZAC_RAWNAME}${ZITI_DOMAIN_SUFFIX}"

export ZITI_EDGE_CONTROLLER_API="${ZITI_EDGE_CONTROLLER_HOSTNAME}:${ZITI_EDGE_CONTROLLER_PORT}"

export ZITI_SIGNING_CERT_NAME="${ZITI_NETWORK}-signing"

export ZITI_CONTROLLER_ROOTCA_NAME="${ZITI_CONTROLLER_HOSTNAME}-root-ca"
export ZITI_CONTROLLER_INTERMEDIATE_NAME="${ZITI_CONTROLLER_HOSTNAME}-intermediate"

export ZITI_EDGE_CONTROLLER_ROOTCA_NAME="${ZITI_EDGE_CONTROLLER_HOSTNAME}-root-ca"
export ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME="${ZITI_EDGE_CONTROLLER_HOSTNAME}-intermediate"

export ZITI_SIGNING_ROOTCA_NAME="${ZITI_SIGNING_CERT_NAME}-root-ca"
export ZITI_SIGNING_INTERMEDIATE_NAME="${ZITI_SIGNING_CERT_NAME}-intermediate"

export ZITI_BIN_DIR="${ZITI_HOME}/ziti-bin"
export ZITI_BIN="${ZITI_BIN_DIR}/ziti"

mkdir -p ${ZITI_BIN_DIR}
mkdir -p ${ZITI_HOME}/db
mkdir -p ${ZITI_PKI}

echo "" > "${ENV_FILE}"
for zEnvVar in $(set -o posix ; set | grep ZITI_ | sort); do echo "export ${zEnvVar}" >> "${ENV_FILE}"; done

export PFXLOG_NO_JSON=true
echo "export PFXLOG_NO_JSON=true" >> "${ENV_FILE}"

echo "alias zec='ziti edge controller'" >> "${ENV_FILE}"
echo "alias zlogin='ziti edge controller login \"${ZITI_EDGE_CONTROLLER_API}\" -u \"${ZITI_USER}\" -p \"${ZITI_PWD}\" -c \"${ZITI_PKI}/${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}.cert\"'" >> "${ENV_FILE}"
echo "alias psz='ps -ef | grep ziti'" >> "${ENV_FILE}"

#when sourcing the emitted file add the bin folder to the path
tee -a "${ENV_FILE}" > /dev/null <<'heredoc'
if [[ "$(echo "$PATH"|grep -q "${ZITI_BIN}" && echo "yes")" == "yes" ]]; then
  echo "${ZITI_BIN} is already on the path"
else
  echo "adding ${ZITI_BIN} to the path"
  export PATH=$PATH:"${ZITI_BIN}"
fi
heredoc