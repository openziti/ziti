#!/usr/bin/env bash
#
# this thin wrapper script for the OpenZiti Controller uses variable assignments from the systemd env file
#

set -o errexit
set -o nounset
set -o pipefail
# set -o xtrace  # debug startup

# shellcheck disable=SC1090 # default path is set by the systemd service
source "${ZITI_CTRL_BOOTSTRAP_BASH:-/opt/openziti/etc/controller/bootstrap.bash}"
# if no args or first arg is "run", bootstrap the router with the config file path as next arg, or default "config.yml"
if [ "${1:-run}" == run ]; then
    bootstrap "${2:-config.yml}"
fi

# shellcheck disable=SC2068
exec ziti controller ${@:-run config.yml}
