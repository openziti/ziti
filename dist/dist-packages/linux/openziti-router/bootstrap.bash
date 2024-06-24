#
# bootstrap the OpenZiti Router with a config file and identity
#

function makeConfig() {
  #
  # create config file
  #

  if [[ ! -s "${ZITI_ROUTER_CONFIG_FILE}" || "${1:-}" == --force ]]; then
    if [[ -z "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" ]]; then
      echo "ERROR: ZITI_CTRL_ADVERTISED_ADDRESS is not set" >&2
      return 1
    fi

    # build config command
    command=("ziti create config router ${ZITI_ROUTER_TYPE}" \
             "--tunnelerMode ${ZITI_ROUTER_MODE}" \
             "--routerName ${ZITI_ROUTER_NAME}" \
             "--output ${ZITI_ROUTER_CONFIG_FILE}")

    # check if ZITI_ROUTER_LAN_INTERFACE is specified and add --lanInterface flag accordingly
    if [[ -n "${ZITI_ROUTER_LAN_INTERFACE}" ]]; then
      command+=("--lanInterface ${ZITI_ROUTER_LAN_INTERFACE}")
    fi

    # execute config command
    # shellcheck disable=SC2068
    ${command[@]}

  fi

}

function enroll() {
  
  # shellcheck disable=SC1090 # find the identity file path
  source <(ziti create config environment | grep ZITI_ROUTER)

  if [[ ! -s "${ZITI_ROUTER_IDENTITY_CERT}" || "${1:-}" == --force ]]; then
    if [ -n "${ZITI_ENROLL_TOKEN:-}" ]; then
      # shellcheck disable=SC2188
      ziti router enroll "${ZITI_ROUTER_CONFIG_FILE}" \
        --jwt <(echo "${ZITI_ENROLL_TOKEN}")
    elif [ -s "/run/credentials/${UNIT_NAME:=ziti-router.service}/ZITI_ENROLL_TOKEN" ]; then
      ziti router enroll "${ZITI_ROUTER_CONFIG_FILE}" \
        --jwt "/run/credentials/${UNIT_NAME}/ZITI_ENROLL_TOKEN"
    else
      echo  "ERROR: use LoadCredential in"\
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
: "${ZITI_ROUTER_ADVERTISED_ADDRESS:=localhost}"
: "${ZITI_ROUTER_NAME:=router}"
: "${ZITI_CTRL_ADVERTISED_PORT:=1280}"
: "${ZITI_ROUTER_PORT:=3022}"
: "${ZITI_ROUTER_MODE:=none}"
export  ZITI_ROUTER_NAME \
        ZITI_ROUTER_ADVERTISED_ADDRESS \
        ZITI_CTRL_ADVERTISED_PORT \
        ZITI_ROUTER_PORT \
        ZITI_ROUTER_LISTENER_BIND_PORT="${ZITI_ROUTER_PORT}"
