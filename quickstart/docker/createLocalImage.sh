#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
ZITI_BIN="${SCRIPT_DIR}/image/ziti-bin"

case "${1:-}" in
  --build)
    mkdir -p "${ZITI_BIN}"
    GOOS="linux" go build -o "${ZITI_BIN}" "${SCRIPT_DIR}/../../..."
    shift
  ;;
esac

###
### This script will recreate the *LATEST* quickstart image locally from *PUBLISHED* binaries
### use this script when you're updating/changing the scripts, file locations inside the container
### to quickly/easily recreate the image for local dev use that CI will publish
###
echo "CREATING latest quickstart containers LOCALLY"
if [ -d "${ZITI_BIN}" ]; then
  echo "rebuilding quickstart using locally built ziti located in ${ZITI_BIN} directory"
  echo ""
else
  echo "rebuilding quickstart using latest ziti from github: no ${ZITI_BIN} directory found"
  echo ""
fi

# optionally, configure ZITI_VERSION for pushLatestDocker.sh
if [[ -n "${ZITI_VERSION_OVERRIDE:-}" && -n "${ZITI_VERSION:-}" ]]; then
  echo "WARN: both ZITI_VERSION and ZITI_VERSION_OVERRIDE are set, overriding $ZITI_VERSION with $ZITI_OVERRIDE_VERSION" >&2
  export ZITI_VERSION="${ZITI_VERSION_OVERRIDE#v}"
elif [[ -n "${ZITI_VERSION_OVERRIDE:-}" ]]; then
  echo "INFO: ZITI_VERSION_OVERRIDE is set, setting ZITI_VERSION=${ZITI_OVERRIDE_VERSION#v}"
  export ZITI_VERSION="${ZITI_VERSION_OVERRIDE#v}"
elif [[ -n "${ZITI_VERSION:-}" ]]; then
  echo "INFO: ZITI_VERSION is set, using ZITI_VERSION=${ZITI_VERSION#v}"
  export ZITI_VERSION="${ZITI_VERSION#v}"
fi

"${SCRIPT_DIR}/pushLatestDocker.sh" local "${1:-}"
