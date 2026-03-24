#!/usr/bin/env bash

# Test v1-to-v2 upgrade path for OpenZiti Linux packages.
# Installs a specified stable v1 from the APT repo, bootstraps controller +
# router, creates test state, then upgrades to locally-built v2 packages and
# verifies:
#   - DynamicUser migration (symlink resolved, ownership fixed, breadcrumb left)
#   - Services restart and controller login still works
#   - Test state survives the upgrade
#
# Usage:
#   dangerous.upgrade.linux.test.bash --from-version <v1-version>
#
# Local:  sudo -i bash /path/to/dangerous.upgrade.linux.test.bash --from-version 1.6.13
# CI:     version resolved in a prior workflow step, passed via --from-version

set -o errexit
set -o nounset
set -o pipefail
set -o errtrace
set -o xtrace

export DEBIAN_FRONTEND=noninteractive

# Handle --help/-h before sourcing the lib, traps, or any destructive setup
for _arg in "$@"; do
  case "$_arg" in
    --help|-h)
      cat <<'EOF'
Usage: dangerous.upgrade.linux.test.bash --from-version <version> [--keep] [--only-clean]

  --from-version <version>  v1 package version to install from APT (e.g., 1.6.13)
  --keep                    keep the test instance running on exit (for inspection)
  --only-clean              run cleanup only (tear down a kept instance) and exit

Re-run with </dev/null to skip the pre-cleanup TTY delay.
EOF
      exit 0
      ;;
  esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=deployments-test-lib.bash
source "${SCRIPT_DIR}/deployments-test-lib.bash"

# Ensure non-zero exit on any failure and always clean up.
# The ERR trap dumps service diagnostics so failures are never silent.
_exit_code=0
_in_err_handler=0
_err_handler() {
  _exit_code=$?
  # Guard against re-entrancy (diagnostics commands may also fail)
  if (( _in_err_handler )); then return; fi
  _in_err_handler=1
  log_error "FAILED at line ${LINENO}: ${BASH_COMMAND} (exit ${_exit_code})"
  # Redirect to stderr so diagnostics don't pollute command substitution stdout
  # (errtrace causes this handler to fire inside $() subshells)
  dump_service_diagnostics ziti-controller.service >&2
  dump_service_diagnostics ziti-router.service >&2
}
trap '_err_handler' ERR

# ============================================================
# Parse required CLI arguments
# ============================================================
usage() {
  trap - EXIT ERR  # don't clean up on usage/help exits
  cat >&2 <<'EOF'
Usage: dangerous.upgrade.linux.test.bash --from-version <version> [--keep] [--only-clean]

  --from-version <version>  v1 package version to install from APT (e.g., 1.6.13)
  --keep                    keep the test instance running on exit (for inspection)
  --only-clean              run cleanup only (tear down a kept instance) and exit
EOF
  exit 1
}

FROM_VERSION=""
ONLY_CLEAN=0
# shellcheck disable=SC2034  # KEEP is checked by cleanup_all in deployments-test-lib.bash
while [[ $# -gt 0 ]]; do
  case "$1" in
    --from-version) FROM_VERSION="$2"; shift 2 ;;
    --keep)         KEEP=1; shift ;;
    --only-clean)   ONLY_CLEAN=1; shift ;;
    *)              usage ;;
  esac
done

if (( ONLY_CLEAN )); then
  trap - EXIT ERR  # cleanup is intentional here; don't double-run on exit
  # shellcheck disable=SC2034  # KEEP is used by cleanup_all in lib
  KEEP=0
  cleanup_all
  log_info "cleanup-only complete"
  exit 0
fi

[[ -n "${FROM_VERSION}" ]] || { trap - EXIT ERR; log_error "--from-version is required"; usage; }

# EXIT trap set here — after all early exits — so cleanup only fires for real test runs
trap 'cleanup_all; exit $_exit_code' EXIT

BASEDIR="${SCRIPT_DIR}"
REPOROOT="$(cd "${BASEDIR}/../../.." && pwd)"
cd "${REPOROOT}"

declare -a BINS=(grep go nc nfpm curl jq)
for BIN in "${BINS[@]}"; do
    check_command "$BIN"
done

# --- Test configuration (hard-coded, piped to v1 bootstrap.bash via stdin) ---
: "${TMPDIR:=$(mktemp -d)}"

ZITI_CTRL_ADVERTISED_ADDRESS="ziti-controller1.127.0.0.1.sslip.io"
ZITI_CTRL_ADVERTISED_PORT="1281"
ZITI_USER="admin"
ZITI_PWD="$(generate_password)"
ZITI_ROUTER_NAME="ziti-router1"
ZITI_ROUTER_ADVERTISED_ADDRESS="${ZITI_ROUTER_NAME}.127.0.0.1.sslip.io"
ZITI_ROUTER_PORT="30223"
ZITI_ENROLL_TOKEN_FILE="${TMPDIR}/${ZITI_ROUTER_NAME}.jwt"

cleanup_all

for PORT in "${ZITI_CTRL_ADVERTISED_PORT}" "${ZITI_ROUTER_PORT}"; do
    check_port_available "${PORT}"
done

# ============================================================
# Phase 1: Install v1 from the stable APT repo
# ============================================================
log_section "Phase 1: Install v1 from APT"

log_info "pinning to v1 version: ${FROM_VERSION}"

setup_openziti_apt_repo
install_from_apt \
  "openziti=${FROM_VERSION}" \
  "openziti-controller=${FROM_VERSION}" \
  "openziti-router=${FROM_VERSION}"

# Verify we actually got v1
_installed_ver=$(dpkg-query -W -f='${Version}' openziti-controller)
log_info "installed openziti-controller version: ${_installed_ver}"
if [[ "${_installed_ver}" != "${FROM_VERSION}" ]]; then
  log_error "expected version ${FROM_VERSION} but got ${_installed_ver}"
  exit 1
fi

# ============================================================
# Phase 2: Bootstrap v1 controller + router
# ============================================================
log_section "Phase 2: Bootstrap v1 controller"

# v1 uses DynamicUser=yes.  bootstrap.bash must run to generate PKI and
# config.yml, but it runs as root — creating files the dynamic UID can't
# read.  Fix: write answers to bootstrap.env (loaded by loadEnvFiles before
# prompts), run bootstrap with /dev/null stdin (non-interactive), stop any
# service it started, open permissions on the state directory, then start
# the service cleanly.
#
# Key: loadEnvFiles runs BEFORE prompts, loadEnvStdin runs AFTER.
# Heredocs feed loadEnvStdin (too late).  bootstrap.env feeds loadEnvFiles.
sudo tee /opt/openziti/etc/controller/bootstrap.env >/dev/null <<CTRL
ZITI_CTRL_ADVERTISED_ADDRESS=${ZITI_CTRL_ADVERTISED_ADDRESS}
ZITI_CTRL_ADVERTISED_PORT=${ZITI_CTRL_ADVERTISED_PORT}
ZITI_USER=${ZITI_USER}
ZITI_PWD=${ZITI_PWD}
CTRL
sudo /opt/openziti/etc/controller/bootstrap.bash </dev/null

# bootstrap.bash may have started the service (and it crash-loops because
# DynamicUser can't read root-owned files).  Stop it, fix permissions, restart.
sudo systemctl stop ziti-controller.service 2>/dev/null || true
sudo systemctl reset-failed ziti-controller.service 2>/dev/null || true
sudo chmod -R a+rX /var/lib/private/ziti-controller/
# database and raft dir need write access
sudo chmod -R a+w /var/lib/private/ziti-controller/

start_service ziti-controller.service
wait_for_service ziti-controller.service 60
wait_for_port "${ZITI_CTRL_ADVERTISED_ADDRESS}" "${ZITI_CTRL_ADVERTISED_PORT}" 30

# shellcheck disable=SC2140
login_cmd="${ZITI_BIN} edge login ${ZITI_CTRL_ADVERTISED_ADDRESS}:${ZITI_CTRL_ADVERTISED_PORT}"\
" --yes"\
" --username ${ZITI_USER}"\
" --password ${ZITI_PWD}"
# shellcheck disable=SC2086  # intentional word splitting for retry args
retry 10 3 ${login_cmd}

log_section "Phase 2: Bootstrap v1 router"
# Create default policies so routers can serve traffic for verify_traffic
"${ZITI_BIN}" edge create edge-router-policy default --edge-router-roles '#all' --identity-roles '#all'
"${ZITI_BIN}" edge create service-edge-router-policy default --edge-router-roles '#all' --service-roles '#all'

"${ZITI_BIN}" edge create edge-router "${ZITI_ROUTER_NAME}" -to "${ZITI_ENROLL_TOKEN_FILE}"

if [[ ! -s "${ZITI_ENROLL_TOKEN_FILE}" ]]; then
    log_error "router enrollment token not found at ${ZITI_ENROLL_TOKEN_FILE}"
    exit 1
fi
ZITI_ENROLL_TOKEN_CONTENT="$(<"${ZITI_ENROLL_TOKEN_FILE}")"
if [[ -z "${ZITI_ENROLL_TOKEN_CONTENT}" ]]; then
    log_error "router enrollment token is empty in ${ZITI_ENROLL_TOKEN_FILE}"
    exit 1
fi

# Same pattern as controller: write to bootstrap.env, run directly, fix perms.
sudo tee /opt/openziti/etc/router/bootstrap.env >/dev/null <<ROUTER
ZITI_CTRL_ADVERTISED_ADDRESS=${ZITI_CTRL_ADVERTISED_ADDRESS}
ZITI_CTRL_ADVERTISED_PORT=${ZITI_CTRL_ADVERTISED_PORT}
ZITI_ROUTER_ADVERTISED_ADDRESS=${ZITI_ROUTER_ADVERTISED_ADDRESS}
ZITI_ROUTER_PORT=${ZITI_ROUTER_PORT}
ZITI_ENROLL_TOKEN=${ZITI_ENROLL_TOKEN_CONTENT}
ROUTER
sudo /opt/openziti/etc/router/bootstrap.bash </dev/null

sudo systemctl stop ziti-router.service 2>/dev/null || true
sudo systemctl reset-failed ziti-router.service 2>/dev/null || true
sudo chmod -R a+rX /var/lib/private/ziti-router/
sudo chmod -R a+w /var/lib/private/ziti-router/

start_service ziti-router.service
wait_for_service ziti-router.service 60

retry 10 3 bash -c "[[ \$($ZITI_BIN edge list edge-routers -j | jq \".data[0].isOnline\") == \"true\" ]]"
log_info "v1 router is online"

# NOTE: verify_traffic is skipped on v1 — the v1 ziti binary does not support
# 'ops verify traffic' (or lacks --yes/session reuse), causing TTY prompts
# that fail in non-interactive mode.  We verify traffic after the v2 upgrade.

# ============================================================
# Phase 3: Create test state that must survive the upgrade
# ============================================================
log_section "Phase 3: Create test state"

# Create an edge router policy so we can verify it survives
"${ZITI_BIN}" edge create edge-router-policy upgrade-test-erp --edge-router-roles '#all' --identity-roles '#all'
"${ZITI_BIN}" edge create service upgrade-test-svc --role-attributes 'upgrade-test'
"${ZITI_BIN}" edge create service-edge-router-policy upgrade-test-serp --service-roles '#upgrade-test' --edge-router-roles '#all'

# Stop services before upgrade (simulate how dpkg postinstall will find them)
stop_service ziti-router.service
stop_service ziti-controller.service

# ============================================================
# Phase 4: Build and install v2 packages (triggers postinstall migration)
# ============================================================
log_section "Phase 4: Upgrade to v2"
build_packages

# Record v1 conffile checksums before upgrade. These are config|noreplace
# in nfpm, so dpkg must preserve the user's modified versions.
_ctrl_svc_env_md5="$(md5sum /opt/openziti/etc/controller/service.env | awk '{print $1}')"
_rtr_svc_env_md5="$(md5sum /opt/openziti/etc/router/service.env | awk '{print $1}')"

# Upgrade without --force-confnew so dpkg preserves user-modified conffiles.
# This is the realistic upgrade path — service.env retains ZITI_BOOTSTRAP=true
# and the admin's original bootstrap answers in bootstrap.env are preserved.
upgrade_local_debs "${TMPDIR}"

# ============================================================
# Phase 5: Verify migration
# ============================================================
log_section "Phase 5: Verify migration"

verify_state_dir ziti-controller ziti-controller ziti-controller
verify_state_dir ziti-router ziti-router ziti-router
verify_symlink_resolved ziti-controller
verify_symlink_resolved ziti-router

# Verify conffiles were preserved (not replaced by package defaults).
# dpkg respects config|noreplace and keeps the user's modified versions.
_verify_conffile() {
  local _path="$1" _expected_md5="$2" _label="$3"
  local _actual_md5
  _actual_md5="$(md5sum "${_path}" | awk '{print $1}')"
  if [[ "${_actual_md5}" == "${_expected_md5}" ]]; then
    log_pass "${_label} preserved across upgrade"
  else
    log_fail "${_label} was replaced during upgrade (expected md5 ${_expected_md5}, got ${_actual_md5})"
    return 1
  fi
}
_verify_conffile /opt/openziti/etc/controller/service.env "${_ctrl_svc_env_md5}" "controller service.env"
_verify_conffile /opt/openziti/etc/router/service.env "${_rtr_svc_env_md5}" "router service.env"

# Verify dpkg saved the new templates as .dpkg-new for admin review
for _f in /opt/openziti/etc/controller/service.env \
          /opt/openziti/etc/router/service.env; do
  if [[ -f "${_f}.dpkg-new" ]]; then
    log_pass "$(basename "${_f}").dpkg-new exists for admin review"
  else
    log_info "no .dpkg-new for $(basename "${_f}") (package template unchanged)"
  fi
done

# ============================================================
# Phase 6: Restart services and verify they work
# ============================================================
log_section "Phase 6: Restart and verify"

restart_service ziti-controller.service
wait_for_service ziti-controller.service 30
wait_for_port "${ZITI_CTRL_ADVERTISED_ADDRESS}" "${ZITI_CTRL_ADVERTISED_PORT}" 30

# shellcheck disable=SC2086  # intentional word splitting for retry args
retry 10 3 ${login_cmd}
log_pass "controller login succeeded after upgrade"

restart_service ziti-router.service
wait_for_service ziti-router.service 20

retry 10 3 bash -c "[[ \$($ZITI_BIN edge list edge-routers -j | jq \".data[0].isOnline\") == \"true\" ]]"
log_pass "router is online after upgrade"

# Verify data plane works on v2 after upgrade
verify_traffic --prefix upgrade-test-v2

# Verify test state survived the upgrade
_erp_count=$("${ZITI_BIN}" edge list edge-router-policies -j 'name="upgrade-test-erp"' | jq '.data | length')
if [[ "${_erp_count}" == "1" ]]; then
    log_pass "edge router policy survived upgrade"
else
    log_fail "edge router policy missing after upgrade (count=${_erp_count})"
    exit 1
fi

_svc_count=$("${ZITI_BIN}" edge list services -j 'name="upgrade-test-svc"' | jq '.data | length')
if [[ "${_svc_count}" == "1" ]]; then
    log_pass "service survived upgrade"
else
    log_fail "service missing after upgrade (count=${_svc_count})"
    exit 1
fi

# ============================================================
# Phase 7: Verify migration completeness
# ============================================================
log_section "Phase 7: Verify migration completeness"

# The migration flag is only created when the DynamicUser layout is detected.
# On CI with v1 packages that already use static users, migration may be
# a no-op. Check conditionally.
if [[ -d "/var/lib/private/ziti-controller" ]]; then
    verify_migration_flag ziti-controller
    verify_private_dir_clean ziti-controller
    verify_config_paths_migrated ziti-controller
else
    log_info "no DynamicUser layout detected for ziti-controller (v1 may already use static users)"
fi

if [[ -d "/var/lib/private/ziti-router" ]]; then
    verify_migration_flag ziti-router
    verify_private_dir_clean ziti-router
    verify_config_paths_migrated ziti-router
else
    log_info "no DynamicUser layout detected for ziti-router (v1 may already use static users)"
fi

# Verify services are running after the upgrade (postinstall should have
# restarted them if migration stopped them)
verify_service_running ziti-controller
verify_service_running ziti-router

# ============================================================
# Phase 8: Verify migration does not recur on subsequent upgrade
# ============================================================
log_section "Phase 8: Verify migration idempotency"

verify_migration_does_not_recur openziti-controller ziti-controller
verify_migration_does_not_recur openziti-router ziti-router

log_section "All upgrade tests passed"
# cleanup runs via EXIT trap
