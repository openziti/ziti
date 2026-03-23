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
  # systemd needs to re-read the unit file before any systemctl calls,
  # otherwise commands in migrateDynamicUser trigger a stale-unit warning
  systemctl daemon-reload
  migrateDynamicUser
  commonActions
  # If migration stopped the service, restart it now that the unit file and
  # state directory are in their final form.
  if [[ "${_MIGRATION_STOPPED_SERVICE:-}" == "true" ]]; then
    echo "INFO: restarting ${SVC_USER}.service after migration"
    systemctl start "${SVC_USER}.service" || true
  fi
}

commonActions() {
  loadEnvFile

  # ensure state directory ownership matches the service user
  if [[ -d "${STATE_DIR}" ]]; then
    chown -R "${SVC_USER}:${SVC_GROUP}" "${STATE_DIR}"
    chmod -R u=rwX,g=,o= "${STATE_DIR}"
  fi
}

# Detect whether the state directory uses the DynamicUser symlink layout from
# v1 packages (DynamicUser=yes).  Returns 0 (true) when migration is needed.
detectDynamicUserState() {
  # Case 1: symlink /var/lib/<svc> -> private/<svc> still exists
  if [[ -L "${STATE_DIR}" ]]; then
    local _target
    _target="$(readlink "${STATE_DIR}")"
    if [[ "${_target}" == "private/${SVC_USER}" \
       || "${_target}" == "/var/lib/private/${SVC_USER}" ]]; then
      return 0
    fi
  fi
  # Case 2: systemd v246+ already resolved the symlink but the private dir
  # still has leftover data
  if [[ -d "/var/lib/private/${SVC_USER}" && -d "${STATE_DIR}" \
     && ! -L "${STATE_DIR}" ]]; then
    return 0
  fi
  return 1
}

# Migrate state from the DynamicUser layout to the static-user layout.
# Safe to call on fresh installs (no-op) and idempotent on repeated runs.
migrateDynamicUser() {
  if ! detectDynamicUserState; then
    return 0
  fi

  echo "INFO: detected DynamicUser state layout for ${SVC_USER}; migrating..."

  # Stop the service before moving files; record this so upgrade() can restart.
  if systemctl is-active --quiet "${SVC_USER}.service" 2>/dev/null; then
    systemctl stop "${SVC_USER}.service" || true
    _MIGRATION_STOPPED_SERVICE=true
  fi

  local _private_dir="/var/lib/private/${SVC_USER}"

  # Case 1: symlink still exists — remove it and move data
  if [[ -L "${STATE_DIR}" ]]; then
    rm -f "${STATE_DIR}"
    if [[ -d "${_private_dir}" ]]; then
      mv "${_private_dir}" "${STATE_DIR}"
    else
      # symlink existed but private dir is gone; create a fresh state dir
      mkdir -p "${STATE_DIR}"
    fi
  fi

  # Case 2: real dir exists and private dir has leftover content
  # (systemd v246+ already moved data back)
  if [[ -d "${_private_dir}" ]]; then
    # If the private dir still has files, merge them (shouldn't happen in
    # practice, but be safe)
    if [[ -n "$(ls -A "${_private_dir}" 2>/dev/null)" ]]; then
      cp -a "${_private_dir}/." "${STATE_DIR}/" 2>/dev/null || true
    fi
    rm -rf "${_private_dir}"
  fi

  # Fix ownership to the static user
  if ! chown -R "${SVC_USER}:${SVC_GROUP}" "${STATE_DIR}"; then
    echo "WARN: failed to chown ${STATE_DIR} to ${SVC_USER}:${SVC_GROUP}" >&2
  fi
  chmod -R u=rwX,g=,o= "${STATE_DIR}"

  # Fix config files that reference the old /var/lib/private/ path
  local _config="${STATE_DIR}/config.yml"
  if [[ -f "${_config}" ]] \
     && grep -q "/var/lib/private/${SVC_USER}" "${_config}"; then
    sed -i "s|/var/lib/private/${SVC_USER}|${STATE_DIR}|g" "${_config}"
    echo "INFO: updated ${_config} paths from /var/lib/private/${SVC_USER} to ${STATE_DIR}"
  fi

  # Leave a breadcrumb README in the old private location
  mkdir -p "${_private_dir}"
  chown root:root "${_private_dir}"
  chmod 0755 "${_private_dir}"
  cat > "${_private_dir}/README.txt" <<README
OpenZiti State Directory Migration
===================================
Date: $(date --utc --iso-8601=seconds)
Package: ${SVC_USER}
Previous layout: DynamicUser (systemd transient user)
New layout: Static user (${SVC_USER}:${SVC_GROUP})

What happened:
  The ${SVC_USER} package migrated from systemd's DynamicUser=yes
  to a static system user. Your state has been moved from:

    /var/lib/private/${SVC_USER}/

  to:

    ${STATE_DIR}/

  File ownership was changed from the transient dynamic UID to the
  persistent ${SVC_USER} user.

Why:
  Static users provide consistent UIDs across hosts, which is required
  for clustered deployments and simplifies backup/restore operations.

This directory is safe to remove.
README

  logger -t "${SVC_USER}" "Migrated state from DynamicUser layout to static user ${SVC_USER}"
  echo "INFO: DynamicUser migration complete for ${SVC_USER}"
}

createUser() {
  # Prefer systemd-sysusers (declarative, handles UID allocation).
  # Fall back to groupadd/useradd if sysusers is unavailable or fails to
  # create the user (observed on some distros where sysusers exits 0 but
  # does not actually provision the account).
  if command -v systemd-sysusers >/dev/null 2>&1; then
    printf 'u %s - "OpenZiti Controller" "%s" /usr/sbin/nologin\n' \
      "${SVC_USER}" "${STATE_DIR}" \
    | systemd-sysusers --replace="/usr/lib/sysusers.d/${SVC_USER}.conf" - \
    || true
  fi
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

# If stdin is a TTY and the controller has no config yet, offer to run
# bootstrap interactively. This fires on fresh install and on upgrade if
# config.yml was deleted.
if [[ -t 0 ]] && [[ ! -f "${STATE_DIR}/config.yml" ]]; then
  read -r -p "Configure ziti-controller now? [Y/n]: " _answer
  case "${_answer,,}" in
    n|no)
      echo "Run /opt/openziti/etc/controller/bootstrap.bash when ready."
      ;;
    *)
      # Run as child process (not exec) so we can catch SIGINT/failure and
      # still exit 0 to dpkg — the package is installed regardless.
      set +o errexit
      /opt/openziti/etc/controller/bootstrap.bash
      _rc=$?
      set -o errexit
      if (( _rc != 0 )); then
        echo "Bootstrap exited with code ${_rc}."
        echo "Re-run: /opt/openziti/etc/controller/bootstrap.bash"
      fi
      ;;
  esac
fi
