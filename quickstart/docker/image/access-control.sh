#!/bin/bash

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
#### Add service policies

# Allow all identities to use any edge router with the "public" attribute
ziti edge create edge-router-policy all-endpoints-public-routers --edge-router-roles "#public" --identity-roles "#all"

# Allow all edge-routers to access all services
ziti edge create service-edge-router-policy all-routers-all-services --edge-router-roles "#all" --service-roles "#all"

touch "${initFile}"

This docker volume has been initialized.