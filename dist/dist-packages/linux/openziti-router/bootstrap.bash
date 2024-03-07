#!/usr/bin/env bash
#
# bootstrap the OpenZiti Router with a config file and identity
#

function makeConfig() {
  #
  # create config file
  #

  if [[ ! -s "./${ZITI_ROUTER_CONFIG_FILE}" || "${1:-}" == --force ]]; then
    ziti create config router "${ZITI_ROUTER_TYPE}" \
      --tunnelerMode "${ZITI_ROUTER_MODE}" \
      --routerName "${ZITI_ROUTER_NAME}" \
      --output "./${ZITI_ROUTER_CONFIG_FILE}"
  fi

}

function enroll() {
  
  # shellcheck disable=SC1090 # find the identity file path
  source <(ziti create config environment | grep ZITI_ROUTER)

  if [[ ! -s "${ZITI_ROUTER_IDENTITY_CERT}" || "${1:-}" == --force ]]; then
    if [ -n "${ZITI_ENROLL_TOKEN:-}" ]; then
      # shellcheck disable=SC2188
      ziti router enroll "./${ZITI_ROUTER_CONFIG_FILE}" \
        --jwt <(echo "${ZITI_ENROLL_TOKEN}")
    elif [ -s "/run/credentials/${UNIT_NAME:=ziti-router.service}/ZITI_ENROLL_TOKEN" ]; then
      ziti router enroll "./${ZITI_ROUTER_CONFIG_FILE}" \
        --jwt "/run/credentials/${UNIT_NAME}/ZITI_ENROLL_TOKEN"
    else
      echo  "ERROR: use SetCredential or LoadCredential in"\
            " /lib/systemd/system/ziti-router.service or set env var ZITI_ENROLL_TOKEN" >&2
    fi
  fi

}

function bootstrap() {
  
  if [ -n "${1:-}" ]; then
    ZITI_ROUTER_CONFIG_FILE="${1}"
  else
    echo "ERROR: no config file path provided" >&2
    return 1
  fi

  # make config file unless it exists if true, set force to overwrite
  if [ "${ZITI_BOOTSTRAP_CONFIG}"   == true ]; then
    makeConfig
  elif [ "${ZITI_BOOTSTRAP_CONFIG}" == force ]; then
    makeConfig --force
  fi

  # enroll unless certificate exists, set "force" to overwrite key and cert (requires new enrollment token)
  if [ "${ZITI_BOOTSTRAP_ENROLLMENT}" == true ]; then
    enroll
  elif [ "${ZITI_BOOTSTRAP_ENROLLMENT}" == force ]; then
    enroll --force
  fi
}

#
# defaults
#

# used by "ziti create config router" and "ziti create config environment" 
: "${ZITI_ROUTER_ADVERTISED_ADDRESS:=${HOSTNAME:=$(hostname -f)}}"
: "${ZITI_ROUTER_NAME:=${HOSTNAME%%.*}}"
: "${ZITI_CTRL_ADVERTISED_PORT:=443}"
export  ZITI_ROUTER_NAME \
        ZITI_ROUTER_ADVERTISED_ADDRESS \
        ZITI_CTRL_ADVERTISED_PORT \
        ZITI_ROUTER_PORT="${ZITI_ROUTER_ADVERTISED_PORT}" \
        ZITI_ROUTER_LISTENER_BIND_PORT="${ZITI_ROUTER_ADVERTISED_PORT}"
