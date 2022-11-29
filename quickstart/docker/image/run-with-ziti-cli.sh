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
initFile="/openziti/access-control.init"
if [[ -f "${initFile}" ]]; then
  echo " "
  echo "*****************************************************"
  echo " docker-compose init file has been detected"
  echo " the initialization of the docker-compose environment has already happened"
  echo " if you wish to allow this volume to be re-initialized, delete the file"
  echo " located at ${initFile}"
  echo "*****************************************************"
  echo " "
  exit 0
fi

# give the controller scripts time to start and create the ziti environment file if running in docker compose 
until $(test -f "${ZITI_HOME}/ziti.env"); do echo "waiting for ziti.env..."; sleep 1; done

# Pause shortly to avoid the intermittent error of reading the file before it's completely done being written to.
sleep 1

. "${ZITI_SCRIPTS}/ziti-cli-functions.sh"

if [[ "${ZITI_CONTROLLER_RAWNAME-}" == "" ]]; then export export ZITI_CONTROLLER_RAWNAME="ziti-controller"; fi
if [[ "${ZITI_EDGE_CONTROLLER_RAWNAME-}" == "" ]]; then export export ZITI_EDGE_CONTROLLER_RAWNAME="ziti-edge-controller"; fi
if [[ "${ZITI_EDGE_ROUTER_RAWNAME-}" == "" ]]; then export export ZITI_EDGE_ROUTER_RAWNAME="${ZITI_NETWORK-}-edge-router"; fi
if [[ "${ZITI_EDGE_ROUTER_PORT-}" == "" ]]; then export ZITI_EDGE_ROUTER_PORT="3022"; fi
if [[ "${ZITI_EDGE_ROUTER_HOSTNAME}" == "" ]]; then export ZITI_EDGE_ROUTER_HOSTNAME="${ZITI_EDGE_ROUTER_RAWNAME}${ZITI_DOMAIN_SUFFIX}"; fi
if [[ "${ZITI_EDGE_ROUTER_ROLES}" == "" ]]; then export ZITI_EDGE_ROUTER_ROLES="${ZITI_EDGE_ROUTER_RAWNAME}"; fi

. "${ZITI_HOME}"/ziti.env

# Wait for the controller
# shellcheck disable=SC2091
until $(curl -s -o /dev/null --fail -k "https://${ZITI_EDGE_CTRL_ADVERTISED_HOST_PORT}"); do
    echo "waiting for https://${ZITI_EDGE_CTRL_ADVERTISED_HOST_PORT}"
    sleep 2
done

echo " "
echo "*****************************************************"

zitiLogin

echo "*****************************************************"

$1

touch "${initFile}"

echo "This docker volume has been initialized."
