#!/usr/bin/env bash

# Shared test library for OpenZiti Linux deployment tests.
# Source this file from test scripts — do not execute directly.

# Guard against direct execution
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  echo "ERROR: this library must be sourced, not executed directly" >&2
  exit 1
fi

# Full path to the ziti binary installed by the openziti package.
# Tests must use this instead of bare "ziti" to ensure we exercise the
# package-managed executable, not something else that happens to be on PATH.
ZITI_BIN=/usr/bin/ziti

# --- Logging and output ---

# All log functions write to stderr so stdout remains clean for return values.

log_section() {
  printf '\n\033[1;36m=== %s ===\033[0m\n\n' "$1" >&2
}

log_info() {
  printf '\033[34mINFO:\033[0m %s\n' "$1" >&2
}

log_warn() {
  printf '\033[33mWARN:\033[0m %s\n' "$1" >&2
}

log_error() {
  printf '\033[31mERROR:\033[0m %s\n' "$1" >&2
}

log_pass() {
  printf '\033[32mPASS:\033[0m %s\n' "$1" >&2
}

log_fail() {
  printf '\033[31mFAIL:\033[0m %s\n' "$1" >&2
}

# --- Random password generation ---

# Generate a random password suitable for test credentials.
# Alphanumeric only — no shell metacharacters that could break unquoted
# expansion in command strings passed to retry().
# Usage: generate_password [length]   (default: 22)
generate_password() {
  local _len="${1:-22}"
  head -c1024 /dev/urandom | LC_ALL=C tr -dc 'A-Za-z0-9' | cut -c "1-${_len}"
}

# --- Retry and wait ---

# retry <max_attempts> <delay_seconds> <command...>
retry() {
  local _max="$1" _delay="$2"
  shift 2
  local _attempts="${_max}"
  until ! (( _attempts )) || "$@"; do
    (( _attempts-- ))
    log_info "waiting ($(( _max - _attempts ))/${_max}): $*"
    sleep "${_delay}"
  done
  if (( ! _attempts )); then
    log_error "command failed after ${_max} attempts: $*"
    return 1
  fi
}

# wait_for_port <host> <port> <timeout_seconds>
wait_for_port() {
  local _host="$1" _port="$2" _timeout="${3:-30}"
  local _deadline=$(( SECONDS + _timeout ))
  while (( SECONDS < _deadline )); do
    if nc -z "${_host}" "${_port}" >/dev/null 2>&1; then
      log_info "port ${_host}:${_port} is reachable"
      return 0
    fi
    sleep 1
  done
  log_error "port ${_host}:${_port} not reachable within ${_timeout}s"
  return 1
}

# wait_for_service <service> <timeout_seconds>
wait_for_service() {
  local _svc="$1" _timeout="${2:-30}"
  if sudo systemd-run \
    --wait --quiet \
    --service-type=oneshot \
    --property="TimeoutStartSec=${_timeout}s" \
    systemctl is-active "${_svc}"; then
    log_info "${_svc} is active"
    return 0
  fi
  log_error "${_svc} is not active; dumping journal:"
  dump_journal "${_svc}" 40
  return 1
}

# --- Prerequisite checks ---

check_command() {
  if ! command -v "$1" &>/dev/null; then
    log_error "this script requires command '$1'. Please install on the search PATH and try again."
    return 1
  fi
}

check_port_available() {
  local _port="$1"
  if nc -zv localhost "${_port}" &>/dev/null; then
    log_error "port ${_port} is already allocated"
    return 1
  fi
  log_info "port ${_port} is available"
}

# --- Package operations ---

# Build openziti + controller + router .debs with nfpm
build_packages() {
  log_section "Building packages"
  mkdir -p ./release
  go build -o ./release/ ./...
  for PKG in openziti{,-controller,-router}; do
    ZITI_HOMEPAGE="https://openziti.io" \
    ZITI_VENDOR="netfoundry" \
    ZITI_MAINTAINER="Maintainers <developers@openziti.org>" \
    MINIMUM_SYSTEMD_VERSION="232" \
    NFPM_ARCH="all" \
    nfpm pkg \
      --packager deb \
      --target "${TMPDIR}" \
      --config "./dist/dist-packages/linux/nfpm-${PKG}.yaml"
  done
}

# Install locally-built .debs (fresh install — replaces all conffiles)
# Usage: install_local_debs <glob_dir>
install_local_debs() {
  local _dir="${1:-${TMPDIR}}"
  log_section "Installing local .deb packages"
  sudo DEBIAN_FRONTEND=noninteractive dpkg --force-confnew --install "${_dir}/openziti_"*.deb </dev/null
  sudo DEBIAN_FRONTEND=noninteractive dpkg --force-confnew --install "${_dir}/openziti-"{controller,router}_*.deb </dev/null
}

# Upgrade to locally-built .debs preserving user-modified conffiles.
# --force-confold tells dpkg to keep the existing config|noreplace files
# (service.env, bootstrap.env) and save the new package version as *.dpkg-new.
# Note: DEBIAN_FRONTEND=noninteractive only affects debconf, not dpkg's own
# conffile prompts — --force-confold is required to suppress the interactive
# Y/I/N/O/D/Z prompt when stdin is not a tty.
# Usage: upgrade_local_debs <glob_dir>
upgrade_local_debs() {
  local _dir="${1:-${TMPDIR}}"
  log_section "Upgrading local .deb packages (preserving conffiles)"
  sudo DEBIAN_FRONTEND=noninteractive dpkg --force-confold --install "${_dir}/openziti_"*.deb </dev/null
  sudo DEBIAN_FRONTEND=noninteractive dpkg --force-confold --install "${_dir}/openziti-"{controller,router}_*.deb </dev/null
}

# Configure APT source for the stable OpenZiti repo
setup_openziti_apt_repo() {
  log_section "Setting up OpenZiti APT repository"

  # Install prerequisites
  sudo apt-get update </dev/null
  sudo DEBIAN_FRONTEND=noninteractive apt-get install -y gnupg curl </dev/null

  # Import GPG key
  curl -fsSL https://get.openziti.io/tun/package-repos.gpg \
    | sudo gpg --batch --yes --dearmor --output /usr/share/keyrings/openziti.gpg
  sudo chmod a+r /usr/share/keyrings/openziti.gpg

  # Add repo source
  local _repo_line="deb [signed-by=/usr/share/keyrings/openziti.gpg] https://packages.openziti.org/zitipax-openziti-deb-stable debian main"
  echo "${_repo_line}" | sudo tee /etc/apt/sources.list.d/openziti-release.list >/dev/null
  sudo apt-get update </dev/null
}

# Install packages from the OpenZiti APT repo.
# Accepts bare names or APT version-pinned specs (e.g., "openziti=1.7.2").
# Usage: install_from_apt <packages...>
install_from_apt() {
  log_section "Installing from APT: $*"
  sudo DEBIAN_FRONTEND=noninteractive apt-get install -y "$@" </dev/null
}

# --- Service management ---

start_service() {
  local _svc="$1"
  log_info "starting ${_svc}"
  if ! sudo systemctl start "${_svc}"; then
    log_error "${_svc} failed to start"
    dump_service_diagnostics "${_svc}"
    return 1
  fi
}

stop_service() {
  log_info "stopping $1"
  sudo systemctl stop "$1" || true
}

restart_service() {
  local _svc="$1"
  log_info "restarting ${_svc}"
  if ! sudo systemctl restart "${_svc}"; then
    log_error "${_svc} failed to restart"
    dump_service_diagnostics "${_svc}"
    return 1
  fi
}

dump_journal() {
  local _svc="$1" _lines="${2:-100}"
  echo "--- journal: ${_svc} (last ${_lines} lines) ---"
  journalctl -xeu "${_svc}" --no-pager -n "${_lines}" || true
  echo "--- end journal: ${_svc} ---"
}

# Full diagnostics for a failed service: unit status, journal, and state dir listing
dump_service_diagnostics() {
  local _svc="$1"
  (
    set +e  # diagnostics must not trigger errexit
    echo ""
    echo "====== DIAGNOSTICS: ${_svc} ======"
    echo ""
    echo "--- systemctl status ---"
    sudo systemctl status "${_svc}" --no-pager -l 2>&1 || true
    echo ""
    dump_journal "${_svc}" 200
    echo ""
    echo "--- state directory ---"
    local _state_dir="/var/lib/${_svc%.service}"
    if [[ -e "${_state_dir}" ]]; then
      ls -la "${_state_dir}/" 2>&1 || true
      if [[ -L "${_state_dir}" ]]; then
        echo "NOTE: ${_state_dir} is a symlink -> $(readlink "${_state_dir}")"
      fi
    else
      echo "${_state_dir} does not exist"
    fi
    echo ""
    echo "--- config files ---"
    local _svc_name="${_svc%.service}"
    local _etc_dir="/opt/openziti/etc/${_svc_name#ziti-}"
    if [[ -d "${_etc_dir}" ]]; then
      ls -la "${_etc_dir}/" 2>&1 || true
    fi
    echo ""
    echo "====== END DIAGNOSTICS: ${_svc} ======"
    echo ""
  )
}

# --- Verification ---

verify_service_active() {
  local _svc="$1"
  if systemctl is-active --quiet "${_svc}"; then
    log_pass "${_svc} is active"
  else
    log_fail "${_svc} is NOT active"
    dump_journal "${_svc}" 40
    return 1
  fi
}

# verify_state_dir <service_name> <user> <group>
verify_state_dir() {
  local _svc="$1" _user="$2" _group="$3"
  local _dir="/var/lib/${_svc}"

  if [[ ! -d "${_dir}" ]]; then
    log_fail "state directory ${_dir} does not exist"
    return 1
  fi

  if [[ -L "${_dir}" ]]; then
    log_fail "state directory ${_dir} is a symlink (should be a real directory)"
    return 1
  fi

  local _owner
  _owner="$(stat -c '%U:%G' "${_dir}")"
  if [[ "${_owner}" == "${_user}:${_group}" ]]; then
    log_pass "state directory ${_dir} owned by ${_owner}"
  else
    log_fail "state directory ${_dir} owned by ${_owner} (expected ${_user}:${_group})"
    return 1
  fi
}

# Assert that the state directory is NOT a symlink (post-migration)
verify_symlink_resolved() {
  local _svc="$1"
  local _dir="/var/lib/${_svc}"
  if [[ -L "${_dir}" ]]; then
    log_fail "${_dir} is still a symlink"
    return 1
  fi
  log_pass "${_dir} is not a symlink"
}

# Check that the migration README breadcrumb exists
verify_readme_breadcrumb() {
  local _svc="$1"
  local _readme="/var/lib/private/${_svc}/README.txt"
  if [[ -s "${_readme}" ]]; then
    log_pass "migration breadcrumb exists: ${_readme}"
  else
    log_fail "migration breadcrumb missing: ${_readme}"
    return 1
  fi
}

# verify_traffic [--prefix <prefix>]
# End-to-end data plane verification using "ziti ops verify traffic".
# Requires a prior "ziti edge login" in the same session. On the first call,
# uses --cleanup to create test resources; subsequent calls reuse them via the
# same --prefix, and the final call (or ERR trap) should clean up.
VERIFY_TRAFFIC_COUNT=0
verify_traffic() {
  local _prefix="install-test"
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --prefix) _prefix="$2"; shift 2 ;;
      *) shift ;;
    esac
  done

  VERIFY_TRAFFIC_COUNT=$(( VERIFY_TRAFFIC_COUNT + 1 ))
  log_section "Verify traffic #${VERIFY_TRAFFIC_COUNT}"

  # First attempt: quiet retries (transient failures during cluster convergence)
  if retry 3 5 "${ZITI_BIN}" ops verify traffic \
      --timeout 11 \
      --prefix "${_prefix}" \
      --yes \
      --cleanup; then
    log_info "traffic OK (#${VERIFY_TRAFFIC_COUNT})"
    return 0
  fi

  # Second attempt: verbose for diagnostics
  if "${ZITI_BIN}" ops verify traffic \
      --timeout 11 \
      --prefix "${_prefix}" \
      --yes \
      --cleanup \
      --verbose; then
    log_info "traffic OK (#${VERIFY_TRAFFIC_COUNT}, retry)"
    return 0
  fi

  log_error "traffic FAILED (#${VERIFY_TRAFFIC_COUNT})"
  return 1
}

# --- nspawn container management ---

# Global state for nspawn containers (initialized by nspawn_ensure_deps)
NSPAWN_DIR=""
NSPAWN_CONTAINERS=()

# Install mmdebstrap and systemd-container if not present, start machined.
nspawn_ensure_deps() {
  if ! command -v mmdebstrap &>/dev/null || ! command -v systemd-nspawn &>/dev/null; then
    log_info "installing mmdebstrap and systemd-container"
    sudo apt-get update </dev/null
    sudo DEBIAN_FRONTEND=noninteractive apt-get install -y mmdebstrap systemd-container </dev/null
  fi
  sudo systemctl start systemd-machined.service || true
  NSPAWN_DIR="$(mktemp -d)"
  log_info "nspawn working directory: ${NSPAWN_DIR}"
}

# nspawn_create_base <base_dir> <deb_dir>
# Create a minimal rootfs with mmdebstrap and install ziti .deb packages.
# The OS layer (mmdebstrap + systemd + dbus) is cached as a tarball for fast
# reuse. Our .deb packages are always installed fresh on top.
#
# Cache location: NSPAWN_CACHE_DIR (default: /var/cache/openziti-test)
# Cache key: nspawn-base-<codename>.tar
# CI: use actions/cache to persist the cache directory between runs
: "${NSPAWN_CACHE_DIR:=/var/cache/openziti-test}"

nspawn_create_base() {
  local _base_dir="$1" _deb_dir="$2"
  local _codename
  # shellcheck source=/etc/os-release
  _codename="$(. /etc/os-release && echo "${VERSION_CODENAME}")"
  local _cache_tar="${NSPAWN_CACHE_DIR}/nspawn-base-${_codename}.tar"

  if [[ -f "${_cache_tar}" ]]; then
    log_info "restoring cached OS rootfs from ${_cache_tar}"
    sudo mkdir -p "${_base_dir}"
    sudo tar xf "${_cache_tar}" -C "${_base_dir}"
  else
    log_info "creating base rootfs (${_codename}) in ${_base_dir}"
    sudo mmdebstrap --variant=minbase --include=systemd,dbus \
      "${_codename}" "${_base_dir}"

    # Save the OS layer for future runs
    sudo mkdir -p "${NSPAWN_CACHE_DIR}"
    log_info "caching OS rootfs to ${_cache_tar}"
    sudo tar cf "${_cache_tar}" -C "${_base_dir}" .
  fi

  # Copy .deb files into the rootfs (use /root/debs, not /tmp — nspawn mounts
  # a private tmpfs on /tmp that hides files placed there from the host)
  sudo mkdir -p "${_base_dir}/root/debs"
  sudo cp "${_deb_dir}"/openziti_*.deb "${_deb_dir}"/openziti-{controller,router}_*.deb \
    "${_base_dir}/root/debs/"

  # Install packages inside (always fresh — debs change every build)
  sudo systemd-nspawn -D "${_base_dir}" --pipe /bin/bash -euxc \
    "dpkg --force-confnew -i /root/debs/openziti_*.deb && \
     dpkg --force-confnew -i /root/debs/openziti-controller_*.deb /root/debs/openziti-router_*.deb" </dev/null

  # Clean up debs
  sudo rm -rf "${_base_dir}/root/debs"
  log_info "base rootfs created"
}

# nspawn_clone <name>
# Clone the base rootfs to a new container directory.
nspawn_clone() {
  local _name="$1"
  log_info "cloning base rootfs to ${_name}"
  sudo cp -a "${NSPAWN_DIR}/base" "${NSPAWN_DIR}/${_name}"
}

# nspawn_boot <name> [extra_nspawn_args...]
# Boot a container in the background and wait for systemd to be ready.
nspawn_boot() {
  local _name="$1"
  shift
  log_info "booting container ${_name}"
  # No --network-* flags: nspawn shares the host network by default.
  # Each container uses unique ports to avoid conflicts.
  sudo systemd-nspawn \
    --boot \
    --directory="${NSPAWN_DIR}/${_name}" \
    --machine="${_name}" \
    "$@" &
  NSPAWN_CONTAINERS+=("${_name}")

  # Wait for systemd to be ready inside (up to 60s)
  local _deadline=$(( SECONDS + 60 ))
  while (( SECONDS < _deadline )); do
    local _state
    _state="$(sudo systemd-run -M "${_name}" --wait --pipe --collect \
      systemctl is-system-running 2>/dev/null)" || true
    if [[ "${_state}" == "running" || "${_state}" == "degraded" ]]; then
      log_info "container ${_name} is ready (${_state})"
      return 0
    fi
    sleep 2
  done
  log_error "container ${_name} did not become ready within 60s"
  return 1
}

# nspawn_exec <name> <cmd...>
# Execute a command inside a running container.
nspawn_exec() {
  local _name="$1"
  shift
  sudo systemd-run -M "${_name}" --wait --pipe --collect "$@"
}

# nspawn_stop <name>
# Poweroff or terminate a container.
nspawn_stop() {
  local _name="$1"
  sudo machinectl poweroff "${_name}" 2>/dev/null \
    || sudo machinectl terminate "${_name}" 2>/dev/null \
    || true
  # Wait briefly for the machine to disappear
  local _deadline=$(( SECONDS + 15 ))
  while (( SECONDS < _deadline )); do
    if ! machinectl show "${_name}" &>/dev/null; then
      return 0
    fi
    sleep 1
  done
  log_warn "container ${_name} did not stop within 15s"
}

# nspawn_cleanup_all
# Stop all tracked containers and remove the working directory.
nspawn_cleanup_all() {
  if [[ -z "${NSPAWN_DIR:-}" ]]; then
    return 0
  fi
  log_info "cleaning up nspawn containers"
  local _name
  for _name in "${NSPAWN_CONTAINERS[@]+"${NSPAWN_CONTAINERS[@]}"}"; do
    nspawn_stop "${_name}"
  done
  if [[ -d "${NSPAWN_DIR}" ]]; then
    sudo rm -rf "${NSPAWN_DIR}"
  fi
  NSPAWN_CONTAINERS=()
  NSPAWN_DIR=""
}

# Dump diagnostics for services inside an nspawn container.
nspawn_dump_diagnostics() {
  local _name="$1"
  (
    set +e
    echo ""
    echo "====== NSPAWN DIAGNOSTICS: ${_name} ======"
    for _svc in ziti-controller.service ziti-router.service; do
      echo "--- ${_name}: ${_svc} status ---"
      sudo systemd-run -M "${_name}" --wait --pipe --collect \
        systemctl status "${_svc}" --no-pager -l 2>&1 || true
      echo "--- ${_name}: ${_svc} journal ---"
      sudo systemd-run -M "${_name}" --wait --pipe --collect \
        journalctl -xeu "${_svc}" --no-pager -n 100 2>&1 || true
    done
    echo "====== END NSPAWN DIAGNOSTICS: ${_name} ======"
    echo ""
  )
}

# --- Cleanup ---

# Set NO_DESTROY=1 (e.g., via --no-destroy) to skip destructive cleanup.
# Useful for post-mortem inspection of a failed test environment.
: "${NO_DESTROY:=0}"

cleanup_service() {
  local _svc="$1"
  (
    set +e
    sudo systemctl stop "${_svc}.service"
    sudo systemctl disable --now "${_svc}.service"
    sudo systemctl reset-failed "${_svc}.service"
    sudo systemctl clean --what=state "${_svc}.service"
  ) || true
}

cleanup_packages() {
  for _pkg in router controller; do
    (
      set +e
      sudo DEBIAN_FRONTEND=noninteractive apt-get remove --yes --purge "openziti-${_pkg}" </dev/null
      if [[ -d "/opt/openziti/etc/${_pkg}" ]]; then
        sudo rm -r "/opt/openziti/etc/${_pkg}"
      fi
    ) || true
  done
}

cleanup_all() {
  if (( NO_DESTROY )); then
    log_info "cleanup skipped (--no-destroy)"
    return 0
  fi
  if [[ -t 0 ]]; then
    local _msg="Purging openziti-controller and openziti-router packages and deleting /var/lib/ziti-controller/ and /var/lib/ziti-router/"
    if [[ ${#NSPAWN_CONTAINERS[@]} -gt 0 ]]; then
      _msg+="; stopping nspawn containers: ${NSPAWN_CONTAINERS[*]}"
    fi
    echo "${_msg} in 30s. Re-run with </dev/null to skip this delay." >&2
    sleep 30
  fi

  # Clean up nspawn containers before host services
  nspawn_cleanup_all

  local _console_dir="${ZITI_CONSOLE_LOCATION:-/opt/openziti/share/console}"

  for _svc in ziti-{router,controller}; do
    cleanup_service "${_svc}"
  done

  cleanup_packages

  if [[ -d "${_console_dir}" ]]; then
    sudo rm -rf "${_console_dir}"
  fi
  log_info "cleanup complete"
}
