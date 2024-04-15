#!/usr/bin/env bash
#
# this thin wrapper script for the OpenZiti Controller uses variable assignments from the systemd env file
#

set -o errexit
set -o nounset
set -o pipefail
# set -o xtrace  # debug startup

if ! (( $# )); then
    # if no args, run the controller with the default config file
    set -- run config.yml
elif [[ "${1}" == run && -z "${2:-}" ]]; then
    # if first arg is "run" and second arg is empty, run the controller with the default config file
    set -- run config.yml
fi

# shellcheck disable=SC1090 # default path is set by the systemd service
source "${ZITI_CTRL_BOOTSTRAP_BASH:-/opt/openziti/etc/controller/bootstrap.bash}"

# if first arg is "run", bootstrap the controller with the config file
if [[ "${1}" == run && "${ZITI_BOOTSTRAP:-}" == true ]]; then
    bootstrap "${2}"
fi

# shellcheck disable=SC2068
exec ziti controller ${@}
