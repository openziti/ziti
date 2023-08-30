#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
ZITI_BIN="${SCRIPT_DIR}/image/ziti-bin"

case "${1:-}" in
  --build)
    mkdir -p "${ZITI_BIN}"
    go build -o "${ZITI_BIN}" "${SCRIPT_DIR}/../../..."
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

"${SCRIPT_DIR}/pushLatestDocker.sh" local "${1:-}"
