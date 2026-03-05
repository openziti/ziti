#!/usr/bin/env bash

# Test fresh install of OpenZiti Linux controller and router packages.
#
# Local:  sudo -i bash /path/to/linux.install-test.bash
# CI:     runs as root with go, nfpm, etc. already on PATH

set -o errexit
set -o nounset
set -o pipefail
set -o errtrace
set -o xtrace

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
  # Dump nspawn container diagnostics if any are running
  local _cname
  for _cname in "${NSPAWN_CONTAINERS[@]+"${NSPAWN_CONTAINERS[@]}"}"; do
    nspawn_dump_diagnostics "${_cname}" >&2
  done
}
trap '_err_handler' ERR
# Parse CLI flags (KEEP is checked by cleanup_all in deployments-test-lib.bash)
# shellcheck disable=SC2034
while [[ $# -gt 0 ]]; do
  case "$1" in
    --keep) KEEP=1; shift ;;
    *) break ;;
  esac
done

trap 'cleanup_all; exit $_exit_code' EXIT

BASEDIR="${SCRIPT_DIR}"
REPOROOT="$(cd "${BASEDIR}/../../.." && pwd)"
cd "${REPOROOT}"

declare -a BINS=(grep go nc nfpm curl jq unzip)
for BIN in "${BINS[@]}"; do
    check_command "$BIN"
done

: "${ZITI_GO_VERSION:=$(grep -E '^go [0-9]+\.[0-9]*' "./go.mod" | cut -d " " -f2)}"
: "${ZITI_PWD:=$(generate_password)}"
: "${TMPDIR:=$(mktemp -d)}"
: "${ZITI_CTRL_ADVERTISED_ADDRESS:="ziti-controller1.127.0.0.1.sslip.io"}"
: "${ZITI_CTRL_ADVERTISED_PORT:="1281"}"
: "${ZITI_BOOTSTRAP:=true}"
: "${ZITI_BOOTSTRAP_CLUSTER:=true}"
: "${ZITI_BOOTSTRAP_CONSOLE:=true}"
: "${ZITI_CLUSTER_NODE_NAME:=${ZITI_CTRL_ADVERTISED_ADDRESS%%.*}}"
: "${ZITI_CLUSTER_TRUST_DOMAIN:=${ZITI_CTRL_ADVERTISED_ADDRESS#*.}}"
: "${ZITI_ROUTER_PORT:="30223"}"
: "${ZITI_ROUTER_NAME:="ziti-router1"}"
: "${ZITI_ROUTER_ADVERTISED_ADDRESS:="${ZITI_ROUTER_NAME}.127.0.0.1.sslip.io"}"
: "${ZITI_ENROLL_TOKEN:="${TMPDIR}/${ZITI_ROUTER_NAME}.jwt"}"
: "${ZITI_CONSOLE_LOCATION:="/opt/openziti/share/consoletest"}"
: "${ZITI_USER:="admin"}"
: "${ZITI_CTRL2_ADVERTISED_ADDRESS:="ziti-controller2.127.0.0.1.sslip.io"}"
: "${ZITI_CTRL2_ADVERTISED_PORT:="1282"}"
: "${ZITI_CTRL3_ADVERTISED_ADDRESS:="ziti-controller3.127.0.0.1.sslip.io"}"
: "${ZITI_CTRL3_ADVERTISED_PORT:="1283"}"
: "${ZITI_RTR2_NAME:="ziti-router2"}"
: "${ZITI_RTR2_PORT:="30224"}"
: "${ZITI_RTR2_ADVERTISED_ADDRESS:="${ZITI_RTR2_NAME}.127.0.0.1.sslip.io"}"
: "${ZITI_RTR3_NAME:="ziti-router3"}"
: "${ZITI_RTR3_PORT:="30225"}"
: "${ZITI_RTR3_ADVERTISED_ADDRESS:="${ZITI_RTR3_NAME}.127.0.0.1.sslip.io"}"

export \
ZITI_GO_VERSION \
ZITI_USER \
ZITI_PWD \
ZITI_CTRL_ADVERTISED_ADDRESS \
ZITI_CTRL_ADVERTISED_PORT \
ZITI_BOOTSTRAP \
ZITI_BOOTSTRAP_CLUSTER \
ZITI_BOOTSTRAP_CONSOLE \
ZITI_CLUSTER_NODE_NAME \
ZITI_CLUSTER_TRUST_DOMAIN \
ZITI_ROUTER_PORT \
ZITI_ROUTER_NAME \
ZITI_ROUTER_ADVERTISED_ADDRESS \
ZITI_ENROLL_TOKEN \
ZITI_CONSOLE_LOCATION

cleanup_all

for PORT in "${ZITI_CTRL_ADVERTISED_PORT}" "${ZITI_ROUTER_PORT}" \
           "${ZITI_CTRL2_ADVERTISED_PORT}" "${ZITI_CTRL3_ADVERTISED_PORT}" \
           "${ZITI_RTR2_PORT}" "${ZITI_RTR3_PORT}"; do
    check_port_available "${PORT}"
done

build_packages
install_local_debs "${TMPDIR}"

# provide dummy console assets before controller bootstrap so /zac/ is configured and served
sudo mkdir -p "${ZITI_CONSOLE_LOCATION}"
sudo tee "${ZITI_CONSOLE_LOCATION}/index.html" <<< "I am ZAC"
sudo chmod -R +rX "${ZITI_CONSOLE_LOCATION}"

# Write controller answer file for non-interactive bootstrap
CTRL_ANSWER_FILE="${TMPDIR}/controller-answers.env"
cat > "${CTRL_ANSWER_FILE}" <<EOF
ZITI_BOOTSTRAP=${ZITI_BOOTSTRAP}
ZITI_BOOTSTRAP_CLUSTER=${ZITI_BOOTSTRAP_CLUSTER}
ZITI_BOOTSTRAP_CONSOLE=${ZITI_BOOTSTRAP_CONSOLE}
ZITI_CLUSTER_NODE_NAME=${ZITI_CLUSTER_NODE_NAME}
ZITI_CLUSTER_TRUST_DOMAIN=${ZITI_CLUSTER_TRUST_DOMAIN}
ZITI_CTRL_ADVERTISED_ADDRESS=${ZITI_CTRL_ADVERTISED_ADDRESS}
ZITI_CTRL_ADVERTISED_PORT=${ZITI_CTRL_ADVERTISED_PORT}
ZITI_PWD=${ZITI_PWD}
ZITI_USER=${ZITI_USER}
ZITI_CONSOLE_LOCATION=${ZITI_CONSOLE_LOCATION}
EOF

# bootstrap.bash now handles:
# 1. PKI generation
# 2. Config file creation
# 3. Starting the controller service
# 4. Cluster initialization (creating default admin)
log_section "Bootstrapping controller"
DEBUG=1 sudo -E /opt/openziti/etc/controller/bootstrap.bash "${CTRL_ANSWER_FILE}" </dev/null

# Verify controller service is running (bootstrap.bash should have started it)
wait_for_service ziti-controller.service 30

# Verify the service user can reach the controller agent
sudo -u ziti-controller "${ZITI_BIN}" agent stats

# Wait for controller port to be reachable
wait_for_port "${ZITI_CTRL_ADVERTISED_ADDRESS}" "${ZITI_CTRL_ADVERTISED_PORT}" 30

# shellcheck disable=SC2140
login_cmd="${ZITI_BIN} edge login ${ZITI_CTRL_ADVERTISED_ADDRESS}:${ZITI_CTRL_ADVERTISED_PORT}"\
" --yes"\
" --username admin"\
" --password ${ZITI_PWD}"
# shellcheck disable=SC2086  # intentional word splitting for retry args
retry 10 3 ${login_cmd}

# Create default policies so routers can serve traffic for verify_traffic
"${ZITI_BIN}" edge create edge-router-policy default --edge-router-roles '#all' --identity-roles '#all'
"${ZITI_BIN}" edge create service-edge-router-policy default --edge-router-roles '#all' --service-roles '#all'

"${ZITI_BIN}" edge create edge-router "${ZITI_ROUTER_NAME}" -to "${ZITI_ENROLL_TOKEN}"

if [[ -z "${ZITI_ENROLL_TOKEN:-}" || ! -s "${ZITI_ENROLL_TOKEN}" ]]; then
    log_error "router enrollment token not found at ${ZITI_ENROLL_TOKEN:-<unset>}"
    exit 1
fi
ZITI_ENROLL_TOKEN_CONTENT="$(<"${ZITI_ENROLL_TOKEN}")"
if [[ -z "${ZITI_ENROLL_TOKEN_CONTENT}" ]]; then
    log_error "router enrollment token is empty in ${ZITI_ENROLL_TOKEN}"
    exit 1
fi
export ZITI_ENROLL_TOKEN="${ZITI_ENROLL_TOKEN_CONTENT}"

# Write router answer file for non-interactive bootstrap
RTR_ANSWER_FILE="${TMPDIR}/router-answers.env"
cat > "${RTR_ANSWER_FILE}" <<EOF
ZITI_BOOTSTRAP=true
ZITI_BOOTSTRAP_ENROLLMENT=true
ZITI_ENROLL_TOKEN=${ZITI_ENROLL_TOKEN}
ZITI_ROUTER_NAME=${ZITI_ROUTER_NAME}
ZITI_ROUTER_ADVERTISED_ADDRESS=${ZITI_ROUTER_ADVERTISED_ADDRESS}
ZITI_ROUTER_PORT=${ZITI_ROUTER_PORT}
EOF

log_section "Bootstrapping router"
ZITI_BOOTSTRAP=true ZITI_BOOTSTRAP_ENROLLMENT=true DEBUG=1 \
    sudo -E /opt/openziti/etc/router/bootstrap.bash "${RTR_ANSWER_FILE}" </dev/null
start_service ziti-router.service
wait_for_service ziti-router.service 20

retry 10 3 bash -c "[[ \$($ZITI_BIN edge list edge-routers -j | jq \".data[0].isOnline\") == \"true\" ]]"
log_info "router is online"

# First scaling event: ctrl1 + rtr1
verify_traffic

# --- Cluster expansion with nspawn containers ---
#
# IMPORTANT: The cluster expansion runs inside a function called from the
# script's top level so that `set -o errexit` is active. Placing this work
# directly inside an `if` body would suppress errexit (bash spec), allowing
# failures to be silently ignored.

_expand_cluster() {
  # Check whether nspawn deps are available (or can be installed).
  # Return early with a warning if not — this is the only conditional
  # guard, so all subsequent commands run with errexit active.
  if ! command -v mmdebstrap &>/dev/null || ! command -v systemd-nspawn &>/dev/null; then
    if ! { sudo apt-get update </dev/null && \
           sudo DEBIAN_FRONTEND=noninteractive apt-get install -y mmdebstrap systemd-container </dev/null; }; then
      log_warn "mmdebstrap/systemd-container not available — skipping cluster expansion test"
      return 0
    fi
  fi

  log_section "Expanding cluster with nspawn containers"

  nspawn_ensure_deps
  nspawn_create_base "${NSPAWN_DIR}/base" "${TMPDIR}"

  for _node_idx in 2 3; do
    _ctrl_name="ziti-controller${_node_idx}"
    _ctrl_addr_var="ZITI_CTRL${_node_idx}_ADVERTISED_ADDRESS"
    _ctrl_addr="${!_ctrl_addr_var}"
    _ctrl_port_var="ZITI_CTRL${_node_idx}_ADVERTISED_PORT"
    _ctrl_port="${!_ctrl_port_var}"
    _rtr_name_var="ZITI_RTR${_node_idx}_NAME"
    _rtr_name="${!_rtr_name_var}"
    _rtr_addr_var="ZITI_RTR${_node_idx}_ADVERTISED_ADDRESS"
    _rtr_addr="${!_rtr_addr_var}"
    _rtr_port_var="ZITI_RTR${_node_idx}_PORT"
    _rtr_port="${!_rtr_port_var}"
    _container="${_ctrl_name}"

    log_section "Bootstrapping ${_container} (${_ctrl_name} + ${_rtr_name})"

    # Clone rootfs and copy primary's PKI
    nspawn_clone "${_container}"
    sudo mkdir -p "${NSPAWN_DIR}/${_container}/ctrl1-pki/root/certs" \
                 "${NSPAWN_DIR}/${_container}/ctrl1-pki/root/keys"
    sudo cp /var/lib/ziti-controller/pki/root/certs/root.cert \
       "${NSPAWN_DIR}/${_container}/ctrl1-pki/root/certs/"
    sudo cp /var/lib/ziti-controller/pki/root/keys/root.key \
       "${NSPAWN_DIR}/${_container}/ctrl1-pki/root/keys/"

    nspawn_boot "${_container}"

    # Write answer file for joiner controller
    local _nspawn_ctrl_answers="${NSPAWN_DIR}/${_container}/ctrl-answers.env"
    sudo tee "${_nspawn_ctrl_answers}" >/dev/null <<NSPAWN_EOF
ZITI_BOOTSTRAP=true
ZITI_BOOTSTRAP_PKI=true
ZITI_BOOTSTRAP_CONFIG=true
ZITI_BOOTSTRAP_DATABASE=true
ZITI_BOOTSTRAP_CLUSTER=false
ZITI_CLUSTER_NODE_PKI=/ctrl1-pki
ZITI_CLUSTER_NODE_NAME=${_ctrl_name}
ZITI_CTRL_ADVERTISED_ADDRESS=${_ctrl_addr}
ZITI_CTRL_ADVERTISED_PORT=${_ctrl_port}
NSPAWN_EOF

    # Bootstrap joiner controller
    nspawn_exec "${_container}" /bin/bash -euxc "
      DEBUG=1 /opt/openziti/etc/controller/bootstrap.bash /ctrl-answers.env </dev/null
    "

    # Start controller (joiner bootstrap does NOT start it)
    nspawn_exec "${_container}" systemctl start ziti-controller.service

    # Wait for controller port
    wait_for_port "${_ctrl_addr}" "${_ctrl_port}" 30

    # Join the cluster (retry — primary may not accept immediately)
    retry 10 3 nspawn_exec "${_container}" \
      /usr/bin/ziti agent cluster add "tls:${ZITI_CTRL_ADVERTISED_ADDRESS}:${ZITI_CTRL_ADVERTISED_PORT}"

    # Create router on primary controller
    "${ZITI_BIN}" edge create edge-router "${_rtr_name}" \
      -to "${TMPDIR}/${_rtr_name}.jwt"
    _jwt_content=$(<"${TMPDIR}/${_rtr_name}.jwt")

    # Write answer file for router in container
    local _nspawn_rtr_answers="${NSPAWN_DIR}/${_container}/rtr-answers.env"
    sudo tee "${_nspawn_rtr_answers}" >/dev/null <<NSPAWN_EOF
ZITI_BOOTSTRAP=true
ZITI_BOOTSTRAP_ENROLLMENT=true
ZITI_ENROLL_TOKEN='${_jwt_content}'
ZITI_ROUTER_NAME=${_rtr_name}
ZITI_ROUTER_ADVERTISED_ADDRESS=${_rtr_addr}
ZITI_ROUTER_PORT=${_rtr_port}
NSPAWN_EOF

    # Bootstrap router in container
    nspawn_exec "${_container}" /bin/bash -euxc "
      DEBUG=1 /opt/openziti/etc/router/bootstrap.bash /rtr-answers.env </dev/null
    "

    # Start router (bootstrap does NOT start it)
    nspawn_exec "${_container}" systemctl start ziti-router.service

    # Verify router online
    retry 10 3 bash -c "[[ \$(${ZITI_BIN} edge list edge-routers -j 'name=\"${_rtr_name}\"' \
      | jq -r '.data[0].isOnline') == 'true' ]]"
    log_info "${_rtr_name} is online"

    # Verify traffic after each scaling event
    verify_traffic
  done

  # Verify full cluster
  log_section "Verifying 3-node cluster"
  retry 5 3 bash -c "[[ \$(sudo -u ziti-controller ${ZITI_BIN} agent cluster list 2>/dev/null | grep -c 'tls:') -ge 3 ]]"
  log_info "cluster has 3 members"

  retry 5 3 bash -c "[[ \$(${ZITI_BIN} edge list edge-routers -j | jq '[.data[] | select(.isOnline)] | length') -eq 3 ]]"
  log_info "all 3 routers online"
}

_expand_cluster

# --- End cluster expansion ---

# verify console is available
log_section "Verifying console"
curl_cmd="curl -skSfw '%{http_code}\t%{url}\n' -o/dev/null \"https://${ZITI_CTRL_ADVERTISED_ADDRESS}:${ZITI_CTRL_ADVERTISED_PORT}/zac/\""
retry 5 3 eval "${curl_cmd}"
eval "${curl_cmd}"

log_section "All install tests passed"
# cleanup runs via EXIT trap
