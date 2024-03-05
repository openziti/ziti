#!/usr/bin/env bash
#
# this thin wrapper script for the OpenZiti Controller enables
# - evaluating arguments from the env file
# - future: bootstrapping a default run environment with PKI and initialized database
#

set -o errexit
set -o nounset
set -o pipefail

# shellcheck disable=SC2068 # because we want to word-split args
exec /opt/openziti/bin/ziti controller run $@
