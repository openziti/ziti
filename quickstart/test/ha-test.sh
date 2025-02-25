#!/usr/bin/env bash

# raise exceptions
set -o errexit
set -o nounset
set -o pipefail

# set defaults
: "${BUILD_DIR:=./build}"
: "${PFXLOG_NO_JSON:=true}"; export PFXLOG_NO_JSON  # disable JSON log format
: "${VERBOSE:=1}"  # 0: no instance logs printed, 1: print instance logs to stdout
BASE_TMP_DIR="/tmp/ha-quickstart-test"
mkdir -p "$BASE_TMP_DIR"
: "${ziti_home:=$(mktemp -d "$BASE_TMP_DIR/ziti_home_XXXXXX")}"

trap 'rm -rf "/tmp/ha-quickstart-test"' EXIT ERR
trap 'echo "Cleaning up..."; [[ -n "${pid1-}" ]] && kill $pid1 2>/dev/null; [[ -n "${pid2-}" ]] && kill $pid2 2>/dev/null; [[ -n "${pid3-}" ]] && kill $pid3 2>/dev/null' EXIT ERR

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
    
    echo "Waiting five seconds for the controller to be chill...."
    sleep 5
    
    while ! "${BUILD_DIR}/ziti" edge login -u admin -p admin "${advertised_host_port}" -y; do
        if (( elapsed >= timeout )); then
            echo "Login failed after $timeout seconds, exiting."
            exit 1
        fi
        echo "Login failed, retrying..."
        sleep 1
        ((elapsed++))
    done
    
    echo "Login to ${advertised_host_port} successful!"
}

function _check_command() {
    if ! command -v "$1" &>/dev/null; then
        echo "ERROR: this script requires ${BINS[*]}, but '$1' is missing." >&2
        $1
    fi
}

function _wait_for_leader() {
  local timeout=10  # Maximum wait time in seconds
  local elapsed=0
  
  while [ "$elapsed" -lt "$timeout" ]; do
    if "${BUILD_DIR}/ziti" ops cluster list | awk -F'│' 'NR>3 {print $3, $5}' | grep -q "true"; then
      echo "Leader found"
      return 0  # Success
    fi
    echo "leader not found. waiting for leader..."
    sleep 1
    ((elapsed++))
  done

  echo "No leader found after $timeout seconds"
  return 1  # Failure
}

declare -a BINS=(awk grep jq "${BUILD_DIR}/ziti")
for BIN in "${BINS[@]}"; do
    _check_command "$BIN"
done

"${BUILD_DIR}/ziti" edge quickstart ha \
    --ctrl-address="127.0.0.1" \
    --router-address="127.0.0.1" \
    --home="${ziti_home}" \
    --trust-domain="quickstart-ha-test" \
    --instance-id="inst1" \
    --ctrl-port="2001" \
    --router-port="3001" \
    > >(while IFS= read -r line; do echo "inst1: $line"; done) 2>&1 &
pid1=$!

_wait_for_controller "2001"
_wait_for_leader

"${BUILD_DIR}/ziti" edge quickstart join \
    --ctrl-address="127.0.0.1" \
    --router-address="127.0.0.1" \
    --home="${ziti_home}" \
    --trust-domain="quickstart-ha-test" \
    --ctrl-port="2002" \
    --router-port="3002" \
    --instance-id="inst2" \
    --cluster-member="tls:127.0.0.1:2001" \
    > >(while IFS= read -r line; do echo "inst2: $line"; done) 2>&1 &
pid2=$!

sleep 3
_wait_for_controller "2002"
_wait_for_leader

"${BUILD_DIR}/ziti" edge quickstart join \
    --ctrl-address="127.0.0.1" \
    --router-address="127.0.0.1" \
    --home="${ziti_home}" \
    --trust-domain="quickstart-ha-test" \
    --ctrl-port="2003" \
    --router-port="3003" \
    --instance-id="inst3" \
    --cluster-member="tls:127.0.0.1:2001" \
    > >(while IFS= read -r line; do echo "inst3: $line"; done) 2>&1 &
pid3=$!

sleep 3
_wait_for_controller "2003"

echo "========================================================="
echo "HA Cluster should now be online"
"${BUILD_DIR}/ziti" ops cluster list
echo ""
echo "Building and running quickstart test"
echo "========================================================="
"${BUILD_DIR}/ziti" ops verify traffic -u admin -p admin --controller-url localhost:2001 -y \
  > >(while IFS= read -r line; do echo "traffic: $line"; done) 2>&1

ZITI_CTRL_EDGE_ADVERTISED_ADDRESS=localhost \
ZITI_CTRL_EDGE_ADVERTISED_PORT=2001 \
ZITI_ROUTER_NAME="router-inst1" \
go test -tags "quickstart manual" ziti/cmd/edge/quickstart_manual_test.go ziti/cmd/edge/quickstart_shared_test.go
test_exit_code=$?

echo "waiting for processes to exit: $pid1 $pid2 $pid3"
kill $pid1 $pid2 $pid3

# Wait for both processes to finish
wait $pid1 $pid2 $pid3

echo "all processes have exited: $pid1 $pid2 $pid3"
echo "Test exited with code $test_exit_code"
exit $test_exit_code  # Fail script if test fails