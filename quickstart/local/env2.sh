#!/bin/bash
# make the quickstart home folder where all the config files, logs, etc will go
export ZITI_HOME=${HOME}/.ziti/quickstart/${network_name}
export ZITI_POSTGRES_HOST="localhost"
export ZITI_NETWORK=${network_name}
export ZITI_USER="admin"
export ZITI_PWD="admin"
export ZITI_DOMAIN_SUFFIX=".ziti.netfoundry.io"
export ZITI_DOMAIN_SUFFIX=""
export ZITI_ID="${ZITI_HOME}/identities.yml"
export ZITI_FAB_MGMT_PORT="10000"
export ZITI_FAB_CTRL_PORT="6262"
export ZITI_CONTROLLER_RAWNAME="${ZITI_NETWORK}-controller"
export ZITI_EDGE_CONTROLLER_RAWNAME="${ZITI_NETWORK}-edge-controller"
export ZITI_ZAC_RAWNAME="${ZITI_NETWORK}-zac"
export ZITI_EDGE_ROUTER_RAWNAME="${ZITI_NETWORK}-edge-router"
export ZITI_EDGE_WSS_ROUTER_RAWNAME="${ZITI_NETWORK}-edge-wss-router"
export ZITI_ROUTER_BR_RAWNAME="${ZITI_NETWORK}-fabric-router-br"
export ZITI_ROUTER_BLUE_RAWNAME="${ZITI_NETWORK}-fabric-router-blue"
export ZITI_ROUTER_RED_RAWNAME="${ZITI_NETWORK}-fabric-router-red"

export ZITI_PKI="${ZITI_HOME}/pki"
export ZITI_CONTROLLER_HOSTNAME="${ZITI_CONTROLLER_RAWAME}${ZITI_DOMAIN_SUFFIX}"
export ZITI_EDGE_CONTROLLER_HOSTNAME="${ZITI_EDGE_CONTROLLER_RAWNAME}${ZITI_DOMAIN_SUFFIX}"
export ZITI_EDGE_CONTROLLER_PORT="1280"
export ZITI_ZAC_HOSTNAME="${ZITI_ZAC_RAWNAME}${ZITI_DOMAIN_SUFFIX}"
export ZITI_EDGE_ROUTER_HOSTNAME="${ZITI_EDGE_ROUTER_RAWNAME}${ZITI_DOMAIN_SUFFIX}"
export ZITI_EDGE_WSS_ROUTER_HOSTNAME="${ZITI_EDGE_WSS_ROUTER_RAWNAME}${ZITI_DOMAIN_SUFFIX}"
export ZITI_SIGNING_CERT_NAME="${ZITI_NETWORK}-signing"
export ZITI_ROUTER_BR_HOSTNAME="${ZITI_ROUTER_BR_RAWNAME}${ZITI_DOMAIN_SUFFIX}"
export ZITI_ROUTER_BLUE_HOSTNAME="${ZITI_ROUTER_BLUE_RAWNAME}${ZITI_DOMAIN_SUFFIX}"
export ZITI_ROUTER_RED_HOSTNAME="${ZITI_ROUTER_RED_RAWNAME}${ZITI_DOMAIN_SUFFIX}"

export ZITI_EDGE_CONTROLLER_API="${ZITI_EDGE_CONTROLLER_HOSTNAME}:${ZITI_EDGE_CONTROLLER_PORT}"

export ZITI_CONTROLLER_ROOTCA_NAME="${ZITI_CONTROLLER_HOSTNAME}-root-ca"
export ZITI_EDGE_CONTROLLER_ROOTCA_NAME="${ZITI_EDGE_CONTROLLER_HOSTNAME}-root-ca"
export ZITI_SIGNING_ROOTCA_NAME="${ZITI_SIGNING_CERT_NAME}-root-ca"

export ZITI_CONTROLLER_INTERMEDIATE_NAME="${ZITI_CONTROLLER_HOSTNAME}-intermediate"
export ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME="${ZITI_EDGE_CONTROLLER_HOSTNAME}-intermediate"
export ZITI_SIGNING_INTERMEDIATE_NAME="${ZITI_SIGNING_CERT_NAME}-intermediate"

mkdir -p ${ZITI_HOME}/db
mkdir -p ${ZITI_PKI}

ENV_FILE="${ZITI_HOME}/${ZITI_NETWORK}.env"
echo "" > "${ENV_FILE}"
for zEnvVar in $(set -o posix ; set | grep ZITI_ | sort); do echo "export ${zEnvVar}" >> "${ENV_FILE}"; done

export PFXLOG_NO_JSON=true
echo "export PFXLOG_NO_JSON=true" >> "${ENV_FILE}"

echo "alias zec='ziti edge controller'" >> "${ENV_FILE}"
echo "alias zlogin='ziti edge controller login \"${ZITI_EDGE_CONTROLLER_API}\" -u \"${ZITI_USER}\" -p \"${ZITI_PWD}\" -c \"${ZITI_PKI}/${ZITI_EDGE_CONTROLLER_ROOTCA_NAME}/certs/${ZITI_EDGE_CONTROLLER_INTERMEDIATE_NAME}.cert\"'" >> "${ENV_FILE}"
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