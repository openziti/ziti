#!/usr/bin/env bash
set -eo pipefail

###
### This script will recreate the *LATEST* quickstart image locally from *PUBLISHED* binaries
### use this script when you're updating/changing the scripts, file locations inside the container
### to quickly/easily recreate the image for local dev use that CI will publish
###
echo "CREATING latest quickstart containers LOCALLY"
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
$SCRIPT_DIR/pushLatestDocker.sh local