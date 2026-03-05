#!/usr/bin/env bash
#
# Entrypoint for the OpenZiti Controller Docker container.
#
# In Docker with ZITI_BOOTSTRAP=true, runs bootstrap() first to generate
# PKI/config/database, then starts the controller.  Without ZITI_BOOTSTRAP,
# starts the controller directly.
#
# Leaf cert renewal is handled by the ziti-controller-cert-renewal.timer
# (Linux) or by the bootstrap() Docker path (ZITI_AUTO_RENEW_CERTS).
#
# usage:
#   entrypoint.bash run config.yml

set -o errexit
set -o nounset
set -o pipefail

# discard debug unless DEBUG
: "${DEBUG:=0}"
if (( DEBUG )); then
  exec 3>&1
  set -o xtrace
else
  exec 3>/dev/null
fi

# default unless args
if ! (( $# )) || [[ "${1}" == run && -z "${2:-}" ]]; then
  set -- run config.yml
fi

# Source bootstrap.bash for function definitions and PKI defaults.
# The source guard in bootstrap.bash (BASH_SOURCE != $0) ensures only functions
# and defaults are loaded — no traps, no debug fd, no bootstrap execution.
# shellcheck disable=SC1090
source "${ZITI_CTRL_BOOTSTRAP_BASH:-/opt/openziti/etc/controller/bootstrap.bash}"

##############################################################################
# Main dispatch
##############################################################################

if [[ "${ZITI_BOOTSTRAP:-}" == true && "${1}" =~ run ]]; then
  # --- Docker/container bootstrap path ---
  # bootstrap() uses env vars for first-run setup (PKI, config, database).
  # On subsequent container restarts, config.yml already exists and bootstrap()
  # detects that — it only renews certs if ZITI_AUTO_RENEW_CERTS=true.
  bootstrap "${2}"

  # If cluster initialization is needed, start controller in background, init, then wait
  if [[ "${ZITI_BOOTSTRAP_CLUSTER:-}" == true || "${ZITI_BOOTSTRAP_DATABASE:-}" == true ]] \
     && [[ -n "${ZITI_PWD:-}" ]]; then
    echo "INFO: starting controller in background for cluster initialization"
    # shellcheck disable=SC2068
    ziti controller ${@} &
    _ctrl_pid=$!

    if waitForAgent "" 30 1; then
      clusterInit ""
    fi

    wait "${_ctrl_pid}"
  else
    # shellcheck disable=SC2068
    exec ziti controller ${@}
  fi

else
  # --- Normal run (no bootstrap) ---
  # shellcheck disable=SC2068
  exec ziti controller ${@}
fi
