#!/bin/bash

# this script is executed during the docker build, after the build context has been copied to /docker.build.context

dest="${1}"

if [ -d /docker.build.context/ziti-bin ]; then
  mv /docker.build.context/ziti-bin/ "${dest}"
else
  source /docker.build.context/ziti-cli-functions.sh
  getZiti
  mv "${ZITI_BIN_DIR}" "${dest}"
fi
