#!/usr/bin/env bash

#
# bootstrap the OpenZiti Router with a config file and an identity.
#

makeConfig() {
  # enforce first argument is a non-empty string that does not begin with "--"
  if [[ -n "${1:-}" && ! "${1}" =~ ^-- ]]; then
    local _config_file="${1}"
    shift
  else
    echo "ERROR: no config file path provided" >&2
    return 1
  fi
  shopt -u nocasematch  # toggle on case-sensitive comparison

  # used by "ziti create config" as controller host
  if [[ -z "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" || -z "${ZITI_ROUTER_ADVERTISED_ADDRESS:-}" ]]; then
    echo "ERROR: ZITI_CTRL_ADVERTISED_ADDRESS and ZITI_ROUTER_ADVERTISED_ADDRESS must be set" >&2
    hintLinuxBootstrap "${PWD}"
    return 1
  else
    echo "DEBUG: controller address is '${ZITI_CTRL_ADVERTISED_ADDRESS}:${ZITI_CTRL_ADVERTISED_PORT}'" >&3
    echo "DEBUG: router address is '${ZITI_ROUTER_ADVERTISED_ADDRESS}:${ZITI_ROUTER_PORT}'" >&3
  fi

  export  ZITI_ROUTER_NAME \
          ZITI_ROUTER_ADVERTISED_ADDRESS \
          ZITI_CTRL_ADVERTISED_PORT \
          ZITI_ROUTER_PORT \
          ZITI_ROUTER_LISTENER_BIND_PORT="${ZITI_ROUTER_PORT}"

  if [[ ! -s "${_config_file}" || "${1:-}" == --force ]]; then
    # build config command
    local -a _command=("ziti create config router ${ZITI_ROUTER_TYPE}" \
              "--routerName ${ZITI_ROUTER_NAME}" \
              "--output ${_config_file}")

    # mode flag not present in fabric command
    if [[ "${ZITI_ROUTER_TYPE}" == edge ]]; then
      _command+=("--tunnelerMode ${ZITI_ROUTER_MODE}")
    fi

    # check if ZITI_ROUTER_LAN_INTERFACE is specified and add --lanInterface flag accordingly
    if [[ -n "${ZITI_ROUTER_LAN_INTERFACE:-}" ]]; then
      _command+=("--lanInterface ${ZITI_ROUTER_LAN_INTERFACE}")
    fi

    # append args if ZITI_BOOTSTRAP_CONFIG_ARGS is not empty
    if [[ -n "${ZITI_BOOTSTRAP_CONFIG_ARGS:-}" ]]; then
      _command+=("${ZITI_BOOTSTRAP_CONFIG_ARGS}")
    fi

    if [[ -s "${_config_file}" && "${1:-}" == --force ]]; then
      echo "INFO: recreating config file: ${_config_file}"
      mv --no-clobber "${_config_file}"{,".${ZITI_BOOTSTRAP_NOW}.old"}
    fi


    exportZitiVars                # export all ZITI_ vars to be used in bootstrap
    # shellcheck disable=SC2068
    ${_command[@]}
  fi

}

enroll() {
  
  if [[ -n "${1:-}" && ! "${1}" =~ ^-- ]]; then
    local _config_file="${1}"
    local _ziti_home
    _ziti_home="$(dirname "${_config_file}")"
    shift
  else
    echo "ERROR: no config file path provided in first param" >&2
    return 1
  fi

  if [[ ! -s "${ZITI_ROUTER_IDENTITY_CERT}" || "${1:-}" == --force ]]; then
    if [[ -n "${ZITI_ENROLL_TOKEN:-}" && ! -f "${ZITI_ENROLL_TOKEN}" ]]; then
      # shellcheck disable=SC2188
      ziti router enroll "${_config_file}" \
        --jwt <(echo "${ZITI_ENROLL_TOKEN}") 2>&1
    elif [[ -n "${ZITI_ENROLL_TOKEN:-}" && -s "${ZITI_ENROLL_TOKEN}" ]]; then
      ziti router enroll "${_config_file}" \
        --jwt "${ZITI_ENROLL_TOKEN}" 2>&1
    else
      echo  "ERROR: set ZITI_ENROLL_TOKEN to enrollment token" >&2
      return 1
    fi
  fi

}

bootstrap() {

  if [[ -n "${1:-}" ]]; then
    local _router_config_file="${1}"
    echo "DEBUG: using config: $(realpath "${_router_config_file}")" >&3
  else
    echo "ERROR: no config file path provided" >&2
    return 1
  fi

  # make config file unless explicitly disabled or it exists, set "force" to overwrite
  if [[ -s "${_router_config_file}" && "${ZITI_BOOTSTRAP_CONFIG}"   != force ]]; then
    echo "INFO: config file exists in $(realpath "${_router_config_file}")"
  elif [[ "${ZITI_BOOTSTRAP_CONFIG}" == true ]]; then
    makeConfig "${_router_config_file}"
  elif [[ "${ZITI_BOOTSTRAP_CONFIG}" == force ]]; then
    makeConfig "${_router_config_file}" --force
  else
    echo "ERROR: unexpected value in ZITI_BOOTSTRAP_CONFIG=${ZITI_BOOTSTRAP_CONFIG}" >&2
    return 1
  fi

  # make database unless explicitly disabled or it exists
  if [[ "${ZITI_BOOTSTRAP_ENROLLMENT}" == true ]]; then
    enroll "${_router_config_file}"
  elif [[ "${ZITI_BOOTSTRAP_ENROLLMENT}" == force ]]; then
    enroll "${_router_config_file}" --force
  fi

}

isInteractive() {
  # return true if interactive and response is not empty
  if [[ "${DEBIAN_FRONTEND:-}" != "noninteractive" && -t 0 ]]; then
    return 0
  else
    return 1
  fi
}

prompt() {
  # return true if interactive and response is not empty
  if isInteractive; then
    read -r -p "$1" response
    if [[ -n "${response:-}" ]]; then
      echo "${response}"
    else
      return 1
    fi
  else
    echo "DEBUG: non-interactive, unable to prompt for answer: '$1'" >&3
    return 1
  fi
}

loadEnvStdin() {
  # if not a tty (stdin is redirected), then slurp answers from stdin, e.g., env
  # assignments like ZITI_ENROLL_TOKEN=abcd1234, one per line
  if [[ ! -t 0 ]]; then
    while read -r line; do
      if [[ "${line:-}" =~ ^ZITI_.*= ]]; then
        eval "${line}"
        setAnswer "${line}" "${SVC_ENV_FILE}" "${BOOT_ENV_FILE}"
      # ignore lines beginning with # and lines containing only zero or more whitespace chars
      elif [[ "${line:-}" =~ ^(#|\\s*?$) ]]; then
        echo "DEBUG: ignoring '${line}'" >&3
        continue
      else
        echo "WARN: ignoring '${line}'; not a ZITI_* env var assignment" >&2
      fi
    done
  fi
}

# shellcheck disable=SC2120
loadEnvFiles() {
  if (( $#))
  then
    local -a _env_files=("${@}")
  else
    local -a _env_files=("${BOOT_ENV_FILE}" "${SVC_ENV_FILE}")
  fi
  for _env_file in "${_env_files[@]}"
  do
    if [[ -s "${_env_file}" ]]
    then
      # shellcheck disable=SC1090
      source "${_env_file}"
    else
      echo "WARN: missing env file '${_env_file}'" >&2
    fi 
  done
}

promptRouterAddress() {
    if [[ -z "${ZITI_ROUTER_ADVERTISED_ADDRESS:-}" ]]; then
        if ZITI_ROUTER_ADVERTISED_ADDRESS="$(prompt "Enter the DNS name or IP address of this router [localhost]: " || echo "localhost")"; then
            setAnswer "ZITI_ROUTER_ADVERTISED_ADDRESS=${ZITI_ROUTER_ADVERTISED_ADDRESS}" "${BOOT_ENV_FILE}"
        else
            echo "WARN: missing ZITI_ROUTER_ADVERTISED_ADDRESS in ${BOOT_ENV_FILE}" >&2
            return 1
        fi
    fi
}

promptEnrollToken() {
    # do nothing if identity file has stuff in it
    if [[ -s "${ZITI_ROUTER_IDENTITY_CERT}" ]]; then
        echo "DEBUG: not prompting for token because identity exists in ${ZITI_HOME}/${ZITI_ROUTER_IDENTITY_CERT}" >&3
    # prompt for enrollment token if interactive, unless already answered
    else
        if ! [[ "${ZITI_BOOTSTRAP_ENROLLMENT:-}" == true ]]; then
            echo "WARN: ZITI_BOOTSTRAP_ENROLLMENT is not true in ${SVC_ENV_FILE}, not enrolling" >&2
        # do nothing if enrollment token is already defined in env file
        elif [[ -n "${ZITI_ENROLL_TOKEN:-}" ]]; then
            echo "DEBUG: ZITI_ENROLL_TOKEN is defined in ${BOOT_ENV_FILE}" >&3
        else
            if ZITI_ENROLL_TOKEN=$(prompt "Router enrollment token as string or path [required]: "); then
                if [[ -n "${ZITI_ENROLL_TOKEN:-}" ]]; then
                    setAnswer "ZITI_ENROLL_TOKEN=${ZITI_ENROLL_TOKEN}" "${BOOT_ENV_FILE}"
                else
                    echo "WARN: missing ZITI_ENROLL_TOKEN in ${BOOT_ENV_FILE}" >&2
                fi
            fi
        fi
    fi
}

grantNetAdmin() {
    # grant ambient capabilities to the router process if not already granted
    if ! grep -qE '^AmbientCapabilities=CAP_NET_ADMIN' "${SVC_FILE}"; then
        # uncomment the line
        sed -Ei 's/.*AmbientCapabilities=CAP_NET_ADMIN/AmbientCapabilities=CAP_NET_ADMIN/' "${SVC_FILE}"
    fi
    systemctl daemon-reload
}

promptRouterPort() {
    # if undefined or default value in env file, prompt for router port, preserving default if no answer
    if [[ -z "${ZITI_ROUTER_PORT:-}" ]]; then
        if ZITI_ROUTER_PORT="$(prompt 'Enter the router port [3022]: ' || echo '3022')"; then
            setAnswer "ZITI_ROUTER_PORT=${ZITI_ROUTER_PORT}" "${BOOT_ENV_FILE}"
        fi
    fi
    if [[ "${ZITI_ROUTER_PORT}" -lt 1024 ]]; then
        grantNetBindService
    fi
}

promptCtrlAddress() {
  if [[ -z "${ZITI_CTRL_ADVERTISED_ADDRESS:-}" ]]; then
    if ! ZITI_CTRL_ADVERTISED_ADDRESS="$(prompt "Enter address of the controller [required]: ")"; then
      echo "ERROR: missing required address ZITI_CTRL_ADVERTISED_ADDRESS in ${BOOT_ENV_FILE}" >&2
      return 1
    else
      setAnswer "ZITI_CTRL_ADVERTISED_ADDRESS=${ZITI_CTRL_ADVERTISED_ADDRESS}" "${BOOT_ENV_FILE}"
    fi
  fi
}

# if bootstrapping was previously explicitly disabled and running interactively then prompt for re-enable, preserving
# the currently disabled setting if non-interactive or no answer
promptBootstrap() {
    # do not prompt if unset or set to true because executing interactively means we want bootstrapping
    if [[ -n "${ZITI_BOOTSTRAP:-}" && "${ZITI_BOOTSTRAP}" != true ]]; then
        if ZITI_BOOTSTRAP="$(prompt 'Generate a default config [y/N]: ' || echo 'false')"; then
            if [[ "${ZITI_BOOTSTRAP}" =~ ^([yY]([eE][sS])?|[tT]([rR][uU][eE])?)$ ]]; then
                ZITI_BOOTSTRAP=true
            elif [[ "${ZITI_BOOTSTRAP}" =~ ^([nN][oO]?|[fF]([aA][lL][sS][eE])?)$ ]]; then
                ZITI_BOOTSTRAP=false
            fi
        fi
        setAnswer "ZITI_BOOTSTRAP=${ZITI_BOOTSTRAP}" "${SVC_ENV_FILE}"
    fi
    if [[ -n "${ZITI_BOOTSTRAP:-}" && "${ZITI_BOOTSTRAP}" != true ]]; then
        return 1
    fi
}

setAnswer() {
  if [[ "${#}" -ge 2 ]]; then
    local _key=${1%=*}
    local _value=${1#*=}
    # strip quotes
    _value="${_value//\"}"
    _value="${_value//\'}"
    shift
    local -a _env_files=("${@}")  # ordered list of files to seek a matching key to assign value
    for _env_file in "${_env_files[@]}"; do
      # do nothing if already set
      if grep -qE "^${_key}=['\"]?${_value}['\"]?[\s$]" "${_env_file}"; then
        return 0
      # set if unset
      elif grep -qE "^${_key}=" "${_env_file}"; then
        sed -Ei "s|^${_key}=.*|${_key}='${_value}'|g" "${_env_file}"
        return 0
      fi
    done
    # append to last file if none matched the key
    echo -e "\n${_key}='${_value}'" >> "${_env_files[${#_env_files[@]}-1]}"
  else
    echo "ERROR: setAnswer() requires at least two arguments, e.g., setAnswer 'ZITI_PWD=abcd1234' ./some1.env ./some2.env" >&2
    return 1
  fi
}

promptCtrlPort() {
  # if undefined or default value in env file, prompt for router port, preserving default if no answer
  if [[ -z "${ZITI_CTRL_ADVERTISED_PORT:-}" ]]; then
    if ZITI_CTRL_ADVERTISED_PORT="$(prompt 'Enter the controller port [1280]: ' || echo '1280')"; then
      setAnswer "ZITI_CTRL_ADVERTISED_PORT=${ZITI_CTRL_ADVERTISED_PORT}" "${BOOT_ENV_FILE}"
    fi
  fi
  if [[ "${ZITI_CTRL_ADVERTISED_PORT}" -lt 1024 ]]; then
    grantNetBindService
  fi
}

grantNetBindService() {
  # grant binding privileged low ports unless already granted
  if ! grep -qE '^AmbientCapabilities=CAP_NET_BIND_SERVICE' "${SVC_FILE}"; then
    # uncomment the line
    sed -Ei 's/.*(AmbientCapabilities=CAP_NET_BIND_SERVICE)/\1/' "${SVC_FILE}"
  fi
  systemctl daemon-reload
}

importZitiVars() {
  # inherit Ziti vars and set answers
  for line in $(set | grep -e "^ZITI_" | sort); do
    # shellcheck disable=SC2013
    setAnswer "${line}" "${SVC_ENV_FILE}" "${BOOT_ENV_FILE}"
  done
}

exportZitiVars() {
  # make ziti vars available in forks
  for line in $(set | grep -e "^ZITI_" | sort); do
    # shellcheck disable=SC2013
    export "${line%=*}"
  done
}

prepareWorkingDir() {
  if [[ -n "${1:-}" ]]; then
    local _config_dir="$1"
    echo "DEBUG: preparing working directory: $(realpath "${_config_dir}")" >&3
  else
    echo "ERROR: no working dir path provided" >&2
    return 1
  fi

  # shellcheck disable=SC2174
  mkdir -pm0700 "${_config_dir}"
  # set pwd for subesquent bootstrap command
  cd "${_config_dir}"
}

finalizeWorkingDir() {
  if [[ -n "${1:-}" ]]; then
    local _config_dir="$1"
    echo "DEBUG: finalizing working directory: $(realpath "${_config_dir}")" >&3
  else
    echo "ERROR: no working dir path provided" >&2
    return 1
  fi

  # disown root to allow systemd to manage the working directory as dynamic user
  chown -R "${ZIGGY_UID:-65534}:${ZIGGY_GID:-65534}" "${_config_dir}/"
  chmod -R u=rwX,go-rwx "${_config_dir}/"
}

hintLinuxBootstrap() {

  local _work_dir="${1:-${PWD}}"

  echo -e "\nProvide a configuration in '${_work_dir}' or generate with:"\
          "\n* Set vars in'/opt/openziti/etc/controller/bootstrap.env'"\
          "\n* Run '/opt/openziti/etc/controller/bootstrap.bash'"\
          "\n* Run 'systemctl enable --now ziti-controller.service'"\
          "\n"
}

dbFile() {
  if ! (( "${#}" )); then
    echo "ERROR: no config file path provided" >&2
    return 1
  fi
  local _config_file="${1}"
  awk -F: '/^db:/ {print $2}' "${_config_file}"|xargs realpath
}

exitHandler() {
  echo "WARN: set VERBOSE=1 or DEBUG=1 for more output" >&2
  if [[ -s "${INFO_LOG_FILE:-}" || -s "${DEBUG_LOG_FILE:-}" ]]; then
    local _log_file
    _log_file="$(mktemp)"
    cat "${INFO_LOG_FILE:-/dev/null}" "${DEBUG_LOG_FILE:-/dev/null}" >| "${_log_file}"
    echo "WARN: see output in '${_log_file}'" >&2
  fi
}

# BEGIN

# discard debug unless this script is executed directly with DEBUG=1
# initialize a file descriptor for debug output
: "${DEBUG:=0}"
: "${VERBOSE:=${DEBUG}}"
if (( DEBUG )); then
  exec 3>&1
  set -o xtrace
else
  exec 3>>"${DEBUG_LOG_FILE:=$(mktemp)}"
fi

trap exitHandler EXIT SIGINT SIGTERM

# set defaults
: "${ZITI_CTRL_ADVERTISED_PORT:=1280}"
: "${ZITI_ROUTER_ADVERTISED_ADDRESS:=localhost}"
: "${ZITI_ROUTER_PORT:=3022}"
: "${ZITI_ROUTER_BIND_ADDRESS:=0.0.0.0}"  # the interface address on which to listen
: "${ZITI_ROUTER_NAME:=router}"  # basename of identity files
: "${ZITI_ROUTER_IDENTITY_CERT:=${ZITI_ROUTER_NAME}.cert}"
: "${ZITI_ROUTER_TYPE:=edge}"  # type of router (edge, fabric)
: "${ZITI_ROUTER_MODE:=host}"  # router will panic if not tunneler-enabled in controller
: "${ZITI_ROUTER_TPROXY_RESOLVER:=udp://127.0.0.1:53}"  # where to listen for DNS requests in tproxy mode
: "${ZITI_ROUTER_DNS_IP_RANGE:=100.64.0.1/10}"  # CIDR range of IP addresses to assign to DNS clients in tproxy mode
ZITI_BOOTSTRAP_NOW="$(date --utc --iso-8601=seconds)"

# if sourced then only define vars and functions and change working directory; else if exec'd then run bootstrap()
if ! [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then

  # ensure ZITI_HOME is working dir to allow paths to be relative or absolute
  cd "${ZITI_HOME:=${PWD}}" || {
    echo "ERROR: failed to cd to '${ZITI_HOME}'" >&2
    exit 1
  }

else

  set -o errexit
  set -o nounset
  set -o pipefail

  export ZITI_HOME=/var/lib/private/ziti-router
  BOOT_ENV_FILE=/opt/openziti/etc/router/bootstrap.env
  SVC_ENV_FILE=/opt/openziti/etc/router/service.env
  SVC_FILE=/etc/systemd/system/ziti-router.service.d/override.conf

  if [[ "${1:-}" =~ ^[-] ]]
  then
    echo -e "\nUsage:"\
            "\n\t$0 [CONFIG_FILE]"\
            "\n" \
            "\nOPTIONS" \
            "\n" \
            "\nVERBOSE=1\tprint INFO" \
            "\nDEBUG=1\t\tprint DEBUG" \
            "\n" >&2
    hintLinuxBootstrap "${ZITI_HOME}"
    exit 1
  elif (( $# ))
  then
    set -- "${ZITI_HOME}/$(basename "$1")"
  else
    set -- "${ZITI_HOME}/config.yml"
  fi
  echo "DEBUG: using config file: $*" >&3

  if [[ $UID != 0 ]]; then
    echo "ERROR: must be run as root" >&2
    exit 1
  fi

  prepareWorkingDir "${ZITI_HOME}"
  loadEnvFiles                  # load lowest precedence vars from SVC_ENV_FILE then BOOT_ENV_FILE
  importZitiVars                # get ZITI_* vars from environment and set in BOOT_ENV_FILE
  loadEnvStdin                  # slurp answers from stdin if it's not a tty
  promptBootstrap               # prompt for ZITI_BOOTSTRAP if explicitly disabled (set and != true)
  promptCtrlAddress             # prompt for ZITI_CTRL_ADVERTISED_ADDRESS if not already set
  promptCtrlPort                # prompt for ZITI_CTRL_ADVERTISED_PORT if not already set
  promptRouterAddress           # prompt for ZITI_ROUTER_ADVERTISED_ADDRESS if not already set
  promptRouterPort              # prompt for ZITI_ROUTER_PORT if not already set
  promptEnrollToken             # prompt for ZITI_ENROLL_TOKEN if not already set
  loadEnvFiles                  # reload env files to source new answers from prompts

  # suppress normal output during bootstrapping unless VERBOSE
  exec 4>&1; exec 1>>"${INFO_LOG_FILE:=$(mktemp)}"
  if (( VERBOSE )); then
    exec 1>&4
  fi
  
  # run bootstrap(), set filemodes, and scrub enroll token
  if bootstrap "${@}"
  then
    finalizeWorkingDir "${ZITI_HOME}"
    setAnswer "ZITI_ENROLL_TOKEN=" "${SVC_ENV_FILE}" "${BOOT_ENV_FILE}"
    # successfully running this script directly means bootstrapping was enabled
    setAnswer "ZITI_BOOTSTRAP=true" "${SVC_ENV_FILE}"
    # if VERBOSE, then stdin was already restore earlier, else do it now to announce completion
    if ! (( VERBOSE )); then
      exec 1>&4
    fi
    echo -e "INFO: bootstrap completed successfully and will not run again."\
            "Adjust ${ZITI_HOME}/config.yml to suit." >&2
    trap - EXIT  # remove exit trap
  else
    echo "ERROR: something went wrong during bootstrapping; set DEBUG=1" >&2
  fi
fi
