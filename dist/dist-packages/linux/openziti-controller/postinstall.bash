#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

# Service user/group — must match the User=/Group= in the systemd unit
SVC_USER="ziti-controller"
SVC_GROUP="${SVC_USER}"
STATE_DIR="/var/lib/${SVC_USER}"

install() {
  createUser
  commonActions
}

upgrade() {
  createUser
  commonActions
  # systemd needs to re-read the unit file after package upgrade
  systemctl daemon-reload
}

commonActions() {
  loadEnvFile

  # ensure state directory ownership matches the service user
  if [[ -d "${STATE_DIR}" ]]; then
    chown -R "${SVC_USER}:${SVC_GROUP}" "${STATE_DIR}"
    chmod -R u=rwX,g=,o= "${STATE_DIR}"
  fi
}

createUser() {
  # Prefer systemd-sysusers (declarative, handles UID allocation).
  # Fall back to groupadd/useradd for older distros.
  if command -v systemd-sysusers >/dev/null 2>&1; then
    printf 'u %s - "OpenZiti Controller" "%s" /usr/sbin/nologin\n' \
      "${SVC_USER}" "${STATE_DIR}" \
    | systemd-sysusers --replace="/usr/lib/sysusers.d/${SVC_USER}.conf" -
  else
    if ! getent group "${SVC_GROUP}" >/dev/null 2>&1; then
      groupadd --system "${SVC_GROUP}"
    fi
    if ! getent passwd "${SVC_USER}" >/dev/null 2>&1; then
      useradd --system \
        --home-dir "${STATE_DIR}" \
        --shell /usr/sbin/nologin \
        --comment "OpenZiti Controller" \
        -g "${SVC_GROUP}" \
        --no-user-group \
        "${SVC_USER}"
    fi
  fi
}

makeEmptyRestrictedFile() {
  if ! [ -s "$1" ]; then
    umask 0177
    touch "$1"
  fi
}

loadEnvFile() {
  # shellcheck disable=SC1090
  if [[ -f "${ZITI_CTRL_SVC_ENV_FILE}" ]]; then
    source "${ZITI_CTRL_SVC_ENV_FILE}"
  fi
  # shellcheck disable=SC1090
  if [[ -f "${ZITI_CTRL_BOOT_ENV_FILE}" ]]; then
    source "${ZITI_CTRL_BOOT_ENV_FILE}"
  fi
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
  if  [[ $1 == 1 || ($1 == configure && -z ${2:-}) ]]; then
    # deb passes $1=configure, rpm passes $1=1
    action=install
  elif [[ $1 == 2 || ($1 == configure && -n ${2:-}) ]]; then
    # deb passes $1=configure $2=<current version>, rpm passes $1=2
    action=upgrade
  else
    echo "ERROR: unexpected action '$1'" >&2
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
    install
    printf "\033[32m completed clean install of openziti-controller\033[0m\n"
    ;;
  "upgrade")
    upgrade
    printf "\033[32m completed upgrade of openziti-controller\033[0m\n"
    ;;
esac
