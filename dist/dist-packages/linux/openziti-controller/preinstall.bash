#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

abort_if_service_exists() {
  if systemctl cat ziti-controller.service &>/dev/null; then
    echo "ERROR: ziti-controller.service is already defined. Please remove it before installing this package." >&2
    (
      set +o errexit
      systemctl cat ziti-controller.service
      echo errexit=$?
    )
    exit 1
  fi
}

commonActions() {
  abort_if_service_exists
}

install() {
  commonActions
}

upgrade() {
  # During upgrade, we don't need to abort if the service exists
  # as this would be the case when upgrading an existing installation
  :
}

# initialize a file descriptor for debug output
: "${DEBUG:=0}"
if (( DEBUG )); then
  exec 3>&1
  set -o xtrace
else
  exec 3>/dev/null
fi

# Step 1, check if this is a clean install or an upgrade
if (( $# )); then
  if  [[ ($1 == 1 || $1 == "install") && -z "${2:-}" ]]; then
    # rpm passes $1=1, deb passes $1=install, neither set $2
    action=install
  elif [[ ($1 == 2 || $1 == "upgrade") && -n "${2:-}" ]]; then
    # rpm passes $1=2 $2=<number of packages to upgrade>, deb passes $1=upgrade $2=<current version>
    action=upgrade
  else
    echo "ERROR: unexpected action '\$1=$1 \$2=$2'" >&2
    exit 1
  fi
else
  echo "ERROR: missing action" >&2
  exit 1
fi

case "$action" in
  "install")
    printf "\033[33m checking for existing ziti-controller service...\033[0m\n"
    install
    ;;
  "upgrade")
    printf "\033[33m processing upgrade of openziti-controller...\033[0m\n"
    upgrade
    ;;
esac
