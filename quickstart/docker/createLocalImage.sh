#!/usr/bin/env bash
set -eo pipefail

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

###
### This script will recreate the *LATEST* quickstart image locally from *PUBLISHED* binaries
### use this script when you're updating/changing the scripts, file locations inside the container
### to quickly/easily recreate the image for local dev use that CI will publish
###
echo "CREATING latest quickstart containers LOCALLY"
if [ -d != "${SCRIPT_DIR}/ziti-bin" ]; then
  echo "rebuilding quickstart using latest ziti from github: no image/ziti-bin directory found"
  echo ""
else
  echo "rebuilding quickstart using locally built ziti located in image/ziti-bin directory"
  echo ""
fi

$SCRIPT_DIR/pushLatestDocker.sh local "${1}"