#!/usr/bin/env bash
#
# Entrypoint for the OpenZiti Router Docker container.
#
# In Docker with ZITI_BOOTSTRAP=true, runs bootstrap() first to enroll and
# generate config, then starts the router.  Without ZITI_BOOTSTRAP, starts
# the router directly.
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

if [[ "${ZITI_BOOTSTRAP:-}" == true && "${1}" =~ run ]]; then
  bootstrap "${2}"
  if [[ "${ZITI_AUTO_RENEW_CERTS:-}" == true && ! "$*" =~ --extend ]]; then
    # shellcheck disable=SC2068
    set -- ${@} --extend
  fi
fi

# shellcheck disable=SC2068
exec ziti router ${@}
