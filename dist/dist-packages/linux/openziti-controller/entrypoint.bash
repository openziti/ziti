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
source "${ZITI_CTRL_BOOTSTRAP_BASH:-/opt/openziti/etc/controller/bootstrap.bash}"

# * Linux service ExecStartPre uses 'check' to pre-flight the config and renew certs
# * Container uses 'run' to call bootstrap() and ziti
if [[ "${ZITI_BOOTSTRAP:-}" == true && "${1}" =~ run ]]; then
  bootstrap "${2}"
elif [[ "${1}" =~ check ]]; then
  if [[ ! -s "${2}" ]]; then
    echo "ERROR: ${2} does not exist" >&2
    hintLinuxBootstrap "${PWD}"
    exit 1
  elif [[ ! -w "$(dbFile "${2}")" ]]; then
    echo "ERROR: database file '$(dbFile "${2}")' is not writable" >&2
    hintLinuxBootstrap "${PWD}"
    exit 1
  elif [[ "${ZITI_BOOTSTRAP:-}" == true && "${ZITI_BOOTSTRAP_PKI:-}" == true ]]; then
    loadEnvFiles /opt/openziti/etc/controller/bootstrap.env
    issueLeafCerts
    exit
  else
    exit
  fi
fi

# shellcheck disable=SC2068
exec ziti controller ${@}
