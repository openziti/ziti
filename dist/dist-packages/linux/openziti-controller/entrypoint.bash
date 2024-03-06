#!/usr/bin/env bash
#
# this thin wrapper script for the OpenZiti Controller uses variable assignments from the systemd env file
#

set -o errexit
set -o nounset
set -o pipefail

# shellcheck disable=SC1091
source /opt/openziti/etc/controller/bootstrap.bash

# shellcheck disable=SC2068 # because we must
# shellcheck disable=SC2086 #  word-split args
exec /opt/openziti/bin/ziti controller run ${ZITI_CONTROLLER_CONFIG_FILE} ${ZITI_CONTROLLER_RUN_ARGS} $@
