#!/usr/bin/env bash

# raise exceptions
set -o errexit
set -o nounset
set -o pipefail

# set defaults
: "${BUILD_DIR:=./build}"
: "${PFXLOG_NO_JSON:=true}"; export PFXLOG_NO_JSON  # disable JSON log format
: "${VERBOSE:=1}"  # 0: no instance logs printed, 1: print instance logs to stdout
declare -a ctrl_ports=(2001 2002 2003)
declare -a router_ports=(3001 3002 3003)
: "${ziti_home:=$(mktemp -d)}"
: "${trust_domain:="quickstart-ha-test"}"

function _wait_for_controller {
    local advertised_host_port="127.0.0.1:${1}"
    local timeout=60
    local elapsed=0

    while [[ 
        "$(curl -w "%{http_code}" -m 1 -sSf -k -o /dev/null \
            https://${advertised_host_port}/edge/client/v1/version 2>/dev/null
        )" != "200" ]]; do
        if (( elapsed >= timeout )); then
            echo "Timeout waiting for https://${advertised_host_port}" >&2
            exit 1
        fi
        echo "waiting for https://${advertised_host_port}"
        sleep 3
        (( elapsed += 3 ))
    done
    echo "CONTROLLER ONLINE AT: https://${advertised_host_port}"
}

function _term_background_pids {
    echo -n "terminating background pids: "
    for name in "${!PIDS[@]}"; do
        echo -n "${name}=${PIDS[$name]} "
    done
    echo -e "\n"
    kill "${PIDS[@]}" 2>/dev/null
    for instance in "${!PIDS[@]}"; do
        while kill -0 "${PIDS[${instance}]}" 2>/dev/null; do
            echo "Waiting for ${instance} process ${PIDS[${instance}]} to stop..."
            sleep 1
        done
        echo "Process ${PIDS[${instance}]} has stopped."
    done
}

function _check_command() {
    if ! command -v "$1" &>/dev/null; then
        echo "ERROR: this script requires ${BINS[*]}, but '$1' is missing." >&2
        $1
    fi
}

declare -a BINS=(awk grep jq "${BUILD_DIR}/ziti")
for BIN in "${BINS[@]}"; do
    _check_command "$BIN"
done

trap '_term_background_pids' EXIT

# initialize an array of instance names
declare -a INSTANCE_NAMES=(inst001 inst002 inst003)
# initialize a map of name=pid
declare -A PIDS

echo "${BUILD_DIR}/ziti" edge quickstart ha \
    --ctrl-address="127.0.0.1" \
    --router-address="127.0.0.1" \
    --home="${ziti_home}" \
    --trust-domain="${trust_domain}" \
    --instance-id="${INSTANCE_NAMES[0]}" \
    --ctrl-port="${ctrl_ports[0]}" \
    --router-port="${router_ports[0]}" \
    > /tmp/ha-test.cmds

nohup "${BUILD_DIR}/ziti" edge quickstart ha \
    --ctrl-address="127.0.0.1" \
    --router-address="127.0.0.1" \
    --home="${ziti_home}" \
    --trust-domain="${trust_domain}" \
    --instance-id="${INSTANCE_NAMES[0]}" \
    --ctrl-port="${ctrl_ports[0]}" \
    --router-port="${router_ports[0]}" \
    &> "${ziti_home}/${INSTANCE_NAMES[0]}.log" &
PIDS["${INSTANCE_NAMES[0]}"]=$!

_wait_for_controller "${ctrl_ports[0]}"
sleep 5
echo "controller online"

echo "${BUILD_DIR}/ziti" edge quickstart join \
    --ctrl-address="127.0.0.1" \
    --router-address="127.0.0.1" \
    --home="${ziti_home}" \
    --trust-domain="${trust_domain}" \
    --ctrl-port="${ctrl_ports[1]}" \
    --router-port="${router_ports[1]}" \
    --instance-id="${INSTANCE_NAMES[1]}" \
    --cluster-member="tls:127.0.0.1:${ctrl_ports[0]}" \
    >> /tmp/ha-test.cmds

nohup "${BUILD_DIR}/ziti" edge quickstart join \
    --ctrl-address="127.0.0.1" \
    --router-address="127.0.0.1" \
    --home="${ziti_home}" \
    --trust-domain="${trust_domain}" \
    --ctrl-port="${ctrl_ports[1]}" \
    --router-port="${router_ports[1]}" \
    --instance-id="${INSTANCE_NAMES[1]}" \
    --cluster-member="tls:127.0.0.1:${ctrl_ports[0]}" \
    &> "${ziti_home}/${INSTANCE_NAMES[1]}.log" &
PIDS["${INSTANCE_NAMES[1]}"]=$!

echo "${BUILD_DIR}/ziti" edge quickstart join \
    --ctrl-address="127.0.0.1" \
    --router-address="127.0.0.1" \
    --home="${ziti_home}" \
    --trust-domain="${trust_domain}" \
    --ctrl-port="${ctrl_ports[2]}" \
    --router-port="${router_ports[2]}" \
    --instance-id="${INSTANCE_NAMES[2]}" \
    --cluster-member="tls:127.0.0.1:${ctrl_ports[0]}" \
    >> /tmp/ha-test.cmds

nohup "${BUILD_DIR}/ziti" edge quickstart join \
    --ctrl-address="127.0.0.1" \
    --router-address="127.0.0.1" \
    --home="${ziti_home}" \
    --trust-domain="${trust_domain}" \
    --ctrl-port="${ctrl_ports[2]}" \
    --router-port="${router_ports[2]}" \
    --instance-id="${INSTANCE_NAMES[2]}" \
    --cluster-member="tls:127.0.0.1:${ctrl_ports[0]}" \
    &> "${ziti_home}/${INSTANCE_NAMES[2]}.log" &
PIDS["${INSTANCE_NAMES[2]}"]=$!

if (( VERBOSE )); then
    # print from the top and follow instance logs with filename separators
    sleep 1; tail -F -n +1 "${ziti_home}/"*.log &
    # add the tail PID to background pids to clean up
    PIDS["logtail"]=$!
fi

count=0
: "${timeout:=60}"  # Timeout in seconds
elapsed=0

while [[ ${count} -lt 3 ]]; do
    results=$("${BUILD_DIR}/ziti" fabric list links -j | jq -r '.data[].state')
    connected_count=$(echo "${results}" | grep -c "Connected" || true)

    if [[ ${connected_count} -eq 3 ]]; then
        echo "All three are connected."
        break
    else
        echo "Waiting for three router links before continuing..."
        sleep 6
        ((elapsed+=6))
        
        if [[ ${elapsed} -ge ${timeout} ]]; then
            "${BUILD_DIR}/ziti" fabric list routers
            "${BUILD_DIR}/ziti" fabric list links
            echo "Timeout reached; not all connections are 'Connected'."
            exit 1
        fi
    fi
done

# three links == things are ready -- tests start below
output=$("${BUILD_DIR}/ziti" agent cluster list --pid "${PIDS["${INSTANCE_NAMES[0]}"]}")

echo ""
echo "${output}"
echo ""

# Extract the columns for LEADER and CONNECTED
leaders=$(echo "${output}" | grep inst | awk -F '│' '{print $5}')
connected=$(echo "${output}" | grep inst | awk -F '/│' '{print $6}')

# Check there is only one leader
leader_count=$(echo "${leaders}" | grep -c "true")
if [[ ${leader_count} -ne 1 ]]; then
    echo "Test failed: Expected 1 leader, found ${leader_count}"
    _term_background_pids
    exit 1
fi

# Check all are connected
disconnected_count=$(echo "${connected}" | grep -c "false" || true)
if [[ ${disconnected_count} -ne 0 ]]; then
    echo "Test failed: Some instances are not connected"
    _term_background_pids
    exit 1
fi

echo "Test passed: One leader found and all instances are connected"
trap - EXIT
_term_background_pids
