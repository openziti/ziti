#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail
# set -o xtrace

# if it exists and is still empty, clean up the enrollment token file that was created by postinstall.bash, allowing the
# package manager to remove the empty directory
ZITI_ENROLL_TOKEN_FILE=/opt/openziti/etc/router/.token
if [ -e "${ZITI_ENROLL_TOKEN_FILE}" ]; then
    if ! [ -s "${ZITI_ENROLL_TOKEN_FILE}" ]; then
        rm -f "${ZITI_ENROLL_TOKEN_FILE}"
    fi
fi
