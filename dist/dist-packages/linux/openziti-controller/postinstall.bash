#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

install() {
  commonActions

}

upgrade() {
  # Step 2(upgrade), do what you need
  commonActions

}

commonActions() {
  loadEnvFile
}

makeEmptyRestrictedFile() {
  if ! [ -s "$1" ]; then
    umask 0177
    touch "$1"
  fi
}

loadEnvFile() {
  # shellcheck disable=SC1090
  [[ -s "${ZITI_CTRL_SVC_ENV_FILE}" ]] && source "${ZITI_CTRL_SVC_ENV_FILE}"
  # shellcheck disable=SC1090
  [[ -s "${ZITI_CTRL_BOOT_ENV_FILE}" ]] && source "${ZITI_CTRL_BOOT_ENV_FILE}"
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
  if  [[ ($1 == 1 || $1 == configure) && -z "${2:-}" ]]; then
    # rpm passes $1=1, deb passes $1=configure
    action=install
  elif [[ ($1 == 2 || $1 == configure) && -n "${2:-}" ]]; then
    # rpm passes $1=2 $2=<number of packages to upgrade>, deb passes $1=configure $2=<current version>
    action=upgrade
  else
    echo "ERROR: unexpected action '\$1=$1 \$2=$2'" >&2
    exit 1
  fi
else
  echo "ERROR: missing action" >&2
  exit 1
fi

ZITI_CTRL_SVC_ENV_FILE=/opt/openziti/etc/controller/service.env
ZITI_CTRL_BOOT_ENV_FILE=/opt/openziti/etc/controller/bootstrap.env

case "$action" in
  "install")
    printf "\033[32m completed clean install of openziti-controller\033[0m\n"
    install
    ;;
  "upgrade")
    printf "\033[32m completed upgrade of openziti-controller\033[0m\n"
    upgrade
    ;;
esac
