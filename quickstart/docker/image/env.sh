#!/bin/bash
export PFXLOG_NO_JSON=true

network_name="ziti"

# make the quickstart home folder where all the config files, logs, etc will go
export ZITI_HOME="/netfoundry/${network_name}"
export ZITI_POSTGRES_HOST="ziti-postgres"
export ZITI_NETWORK=${network_name}
export ZITI_USER="admin"
export ZITI_PWD="admin"
export ZITI_DOMAIN_SUFFIX=".ziti.netfoundry.io"
export ZITI_DOMAIN_SUFFIX=""
export ZITI_ID="${ZITI_HOME}/identities.yml"
export ZITI_FAB_MGMT_PORT="10000"
export ZITI_FAB_CTRL_PORT="6262"
export ZITI_CONTROLLER_NAME="controller"
export ZITI_EDGE_CONTROLLER_NAME="edge-controller"
export ZITI_EDGE_ROUTER_NAME="edge-router"
export ZITI_EDGE_CONTROLLER_PORT="1280"
export ZITI_ROUTER_BR_NAME="fabric-router-br"
export ZITI_ROUTER_BLUE_NAME="fabric-router-blue"
export ZITI_ROUTER_RED_NAME="fabric-router-red"

export ZITI_PKI="${ZITI_HOME}/pki"
export ZITI_EDGE_CONTROLLER_HOSTNAME="${ZITI_NETWORK}-${ZITI_EDGE_CONTROLLER_NAME}${ZITI_DOMAIN_SUFFIX}"
export ZITI_EDGE_ROUTER_HOSTNAME="${ZITI_NETWORK}-${ZITI_EDGE_ROUTER_NAME}${ZITI_DOMAIN_SUFFIX}"
export ZITI_SIGNING_CERT_NAME="${ZITI_NETWORK}-signing"
export ZITI_ROUTER_BR_HOSTNAME="${ZITI_NETWORK}-${ZITI_ROUTER_BR_NAME}${ZITI_DOMAIN_SUFFIX}"
export ZITI_ROUTER_BLUE_HOSTNAME="${ZITI_NETWORK}-${ZITI_ROUTER_BLUE_NAME}${ZITI_DOMAIN_SUFFIX}"
export ZITI_ROUTER_RED_HOSTNAME="${ZITI_NETWORK}-${ZITI_ROUTER_RED_NAME}${ZITI_DOMAIN_SUFFIX}"

export ZITI_EDGE_API_HOSTNAME="${ZITI_NETWORK}-${ZITI_EDGE_CONTROLLER_NAME}:${ZITI_EDGE_CONTROLLER_PORT}"

export ZITI_CONTROLLER_ROOTCA_NAME="${ZITI_EDGE_CONTROLLER_HOSTNAME}-root-ca"
export ZITI_EDGE_ROOTCA_NAME="${ZITI_EDGE_ROUTER_HOSTNAME}-root-ca"
export ZITI_SIGNING_ROOTCA_NAME="${ZITI_SIGNING_CERT_NAME}-root-ca"

export ZITI_CONTROLLER_INTERMEDIATE_NAME="${ZITI_EDGE_CONTROLLER_HOSTNAME}-intermediate"
export ZITI_EDGE_INTERMEDIATE_NAME="${ZITI_EDGE_ROUTER_HOSTNAME}-intermediate"
export ZITI_SIGNING_INTERMEDIATE_NAME="${ZITI_SIGNING_CERT_NAME}-intermediate"

export ZITI_CLUSTER_NAME="${ZITI_NETWORK}-cluster"

mkdir -p ${ZITI_HOME}/db
mkdir -p ${ZITI_PKI}

cat > ziti.env << HereDocForZitiEnv
export PFXLOG_NO_JSON=true
export ZITI_USER="admin"
export ZITI_PWD="admin"
export ZITI_HOME=${ZITI_HOME}
export ZITI_NETWORK=${ZITI_NETWORK}
export ZITI_DOMAIN_SUFFIX="${ZITI_DOMAIN_SUFFIX}"
export ZITI_POSTGRES_HOST="${ZITI_POSTGRES_HOST}"
export ZITI_ID="${ZITI_ID}"
export ZITI_PKI="${ZITI_PKI}"
export ZITI_EDGE_CONTROLLER_HOSTNAME="${ZITI_EDGE_CONTROLLER_HOSTNAME}"
export ZITI_FAB_MGMT_PORT="${ZITI_FAB_MGMT_PORT}"
export ZITI_FAB_CTRL_PORT="${ZITI_FAB_CTRL_PORT}"
export ZITI_CONTROLLER_NAME="${ZITI_CONTROLLER_NAME}"
export ZITI_EDGE_CONTROLLER_NAME="${ZITI_EDGE_CONTROLLER_NAME}"
export ZITI_EDGE_ROUTER_NAME="${ZITI_EDGE_ROUTER_NAME}"
export ZITI_ROUTER_BR_NAME="${ZITI_ROUTER_BR_NAME}"
export ZITI_ROUTER_BLUE_NAME="${ZITI_ROUTER_BLUE_NAME}"
export ZITI_ROUTER_RED_NAME="${ZITI_ROUTER_RED_NAME}"
export ZITI_EDGE_ROUTER_HOSTNAME="${ZITI_EDGE_ROUTER_HOSTNAME}"
export ZITI_ROUTER_BR_HOSTNAME="${ZITI_ROUTER_BR_HOSTNAME}"
export ZITI_ROUTER_BLUE_HOSTNAME="${ZITI_ROUTER_BLUE_HOSTNAME}"
export ZITI_ROUTER_RED_HOSTNAME="${ZITI_ROUTER_RED_HOSTNAME}"
export ZITI_EDGE_CONTROLLER_PORT="${ZITI_EDGE_CONTROLLER_PORT}"
export ZITI_EDGE_API_HOSTNAME="${ZITI_EDGE_API_HOSTNAME}"

export ZITI_CONTROLLER_ROOTCA_NAME=${ZITI_CONTROLLER_ROOTCA_NAME}
export ZITI_EDGE_ROOTCA_NAME=${ZITI_EDGE_ROOTCA_NAME}
export ZITI_SIGNING_ROOTCA_NAME="${ZITI_SIGNING_ROOTCA_NAME}"

export ZITI_CONTROLLER_INTERMEDIATE_NAME="${ZITI_CONTROLLER_INTERMEDIATE_NAME}"
export ZITI_EDGE_INTERMEDIATE_NAME="${ZITI_EDGE_INTERMEDIATE_NAME}"
export ZITI_SIGNING_INTERMEDIATE_NAME="${ZITI_SIGNING_INTERMEDIATE_NAME}"
export ZITI_CLUSTER_NAME="${ZITI_CLUSTER_NAME}"
mkdir -p ${ZITI_HOME}/db
mkdir -p ${ZITI_PKI}
HereDocForZitiEnv




















cat > ziti.env.old << HereDocForZitiEnv
export PFXLOG_NO_JSON=true
export ZITI_USER="admin"
export ZITI_PWD="admin"
export network_name="${network_name}"
export ZITI_HOME="${ZITI_HOME}"
export pki_root="${pki_root}"
export ca_name="${network_name}-root-ca"
export intermediate_name="${network_name}-intermediate"
#export gateway_name="${network_name}-gateway"
export gateway_name="${network_name}-gateway"
export edge_controller_name="${edge_controller_name}"
export edge_controller_port="${edge_controller_port}"
export edge_controller_intermediate="${edge_controller_name}-intermediate"
export edge_controller_uri="https://${edge_controller_name}:${edge_controller_port}"
export fabric_controller_name="${network_name}-fabric-controller"
export fabric_controller_gw_port=10080
export fabric_controller_port="${fabric_controller_port}"
export fabric_controller_gw_name="${network_name}-fabric-gw"
export postgres_host="${postgres_host}"
export dotzeet_identity="${network_name}-dotzeet"
export identity_intermediate="${intermediate_name}-identities"
export gateways_intermediate="${intermediate_name}-gateways"

export cluster_name="${network_name}-cluster"

HereDocForZitiEnv
