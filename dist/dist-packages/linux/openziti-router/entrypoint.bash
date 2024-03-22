#!/usr/bin/env bash
#
# this thin wrapper script for the OpenZiti Router uses variable assignments from the systemd env file
#

set -o errexit
set -o nounset
set -o pipefail

if ! (( $# )); then
    # if no args, run the router with the default config file
    set -- run config.yml
elif [[ "${1}" == run && -z "${2:-}" ]]; then
    # if first arg is "run" and second arg is empty, run the router with the default config file
    set -- run config.yml
fi

# shellcheck disable=SC1090 # default path is assigned in env file
source "${ZITI_ROUTER_BOOTSTRAP_BASH:-/opt/openziti/etc/router/bootstrap.bash}"

# if first arg is "run", bootstrap the router with the config file
if [ "${1}" == run ]; then
    bootstrap "${2}"
fi

# optionally renew certs at startup
if [ "${ZITI_AUTO_RENEW_CERTS:-}" == true ]; then
    # shellcheck disable=SC2068
    set -- ${@} --extend
fi

# shellcheck disable=SC2068
exec ziti router ${@}
