#!/usr/bin/env bash
#
# This wrapper is used by the systemd service and Docker container to call the bootstrap() function before invoking
# ziti. The bootstrap() function will exit 0 immediately if ZITI_BOOTSTRAP is not set to true. Otherwise,  it will
# generate any necessary things that don't already exist.
#
# usage:
#   entrypoint.bash run config.yml

set -o errexit
set -o nounset
set -o pipefail

# discard debug unless DEBUG
: "${DEBUG:=0}"
if (( DEBUG )); then
  exec 3>&1
  set -o xtrace
else
  exec 3>/dev/null
fi

# default unless args
if ! (( $# )) || [[ "${1}" == run && -z "${2:-}" ]]; then
  set -- run config.yml
fi

# shellcheck disable=SC1090 # path is set by the systemd service and Dockerfile
source "${ZITI_ROUTER_BOOTSTRAP_BASH:-/opt/openziti/etc/router/bootstrap.bash}"

# * Linux service ExecStartPre uses 'check' to pre-flight the config and renew certs
# * Container uses 'run' to call bootstrap() and ziti
if [[ "${ZITI_BOOTSTRAP:-}" == true && "${1}" =~ run ]]; then
  bootstrap "${2}"
  if [[ "${ZITI_AUTO_RENEW_CERTS:-}" == true && ! "$*" =~ --extend ]]; then
    # shellcheck disable=SC2068
    set -- ${@} --extend
  fi
elif [[ "${1}" =~ check ]]; then
  if [[ ! -s "${2}" ]]; then
    echo "ERROR: ${2} is empty or missing" >&2
    hintLinuxBootstrap "${PWD}"
    exit 1
  else
    # if check mode and config.yml exists then noop
    echo "DEBUG: ${2} exists and is not empty" >&3
    exit 0
  fi
fi

# shellcheck disable=SC2068
exec ziti router ${@}
