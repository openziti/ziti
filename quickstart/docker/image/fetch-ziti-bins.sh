#!/bin/bash

# this script is executed during the docker build, after the build context has been copied to /docker.build.context

dest="${1}"

if [ -d /docker.build.context/ziti-bin -a ]; then
  mv /docker.build.context/ziti-bin/ "${dest}"
else
   ZITI_BIN_ROOT="${dest}"
   ZITI_BIN_DIR="${ZITI_BIN_ROOT}"
   source /docker.build.context/ziti-cli-functions.sh
   getZiti
fi
