#!/bin/bash

#
# Base script for anything that requires the Ziti CLI
#
if [ "$#" -ne 1 ]; then
  echo "Usage: $0 scriptName"
  exit 0
fi

if [[ ! -f $1 ]]; then
  echo "Script $1 not found"
  echo "Usage: $0 scriptName"
  exit 0
fi

# Should we execute?
initFile="${ZITI_HOME}/access-control.init"
if [[ -f "${initFile}" ]]; then
  echo " "
  echo "*****************************************************************"
  echo " docker-compose init file has been detected, the initialization "
  echo " of the docker-compose environment has already happened. If you "
  echo " wish to allow this volume to be re-initialized, delete the file "
  echo " located at ${initFile}"
  echo "*****************************************************************"
  echo " "
  exit 0
fi

# give the controller scripts time to start and create the ziti environment file if running in docker compose 
until $(test -f "${ZITI_HOME}/ziti.env"); do echo "waiting for ziti.env..."; sleep 1; done

# Pause shortly to avoid the intermittent error of reading the file before it's completely done being written to.
sleep 1

. "${ZITI_SCRIPTS}/ziti-cli-functions.sh"

if [[ "${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS-}" == "" ]]; then export ZITI_CTRL_EDGE_ADVERTISED_ADDRESS="ziti-edge-controller"; fi
if [[ "${ZITI_ROUTER_NAME-}" == "" ]]; then export ZITI_ROUTER_NAME="${ZITI_NETWORK-}-edge-router"; fi
if [[ "${ZITI_ROUTER_PORT-}" == "" ]]; then export ZITI_ROUTER_PORT="3022"; fi
if [[ "${ZITI_ROUTER_ADVERTISED_ADDRESS}" == "" ]]; then export ZITI_ROUTER_ADVERTISED_ADDRESS="${ZITI_ROUTER_NAME}${ZITI_DOMAIN_SUFFIX}"; fi
if [[ "${ZITI_ROUTER_ROLES}" == "" ]]; then export ZITI_ROUTER_ROLES="${ZITI_ROUTER_NAME}"; fi

. "${ZITI_HOME}"/ziti.env

# Wait for the controller
_wait_for_controller

echo " "
echo "*****************************************************"

zitiLogin

echo "*****************************************************"

$1

touch "${initFile}"

echo "This docker volume has been initialized."
