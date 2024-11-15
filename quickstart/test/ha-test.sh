BUILD_DIR=/tmp/build

ctrl_port=2001
router_port=3001
rm -rf "/tmp/quickstart-ha-test"
ziti_home="/tmp/quickstart-ha-test"

function _wait_for_controller {
    local advertised_host_port="127.0.0.1:${1}"
    local timeout=60
    local elapsed=0

    while [[ "$(curl -w "%{http_code}" -m 1 -s -k -o /dev/null https://${advertised_host_port}/edge/client/v1/version)" != "200" ]]; do
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

trap 'kill $inst001pid $inst002pid $inst003pid 2>/dev/null' EXIT

"${BUILD_DIR}/ziti" edge quickstart ha \
    --home "${ziti_home}" \
    --trust-domain="quickstart-ha-test" \
    --instance-id inst001 \
    --ctrl-port "${ctrl_port}" \
    --router-port "${router_port}" \
    &
inst001pid=$!

_wait_for_controller "${ctrl_port}"
sleep 5
echo "controller online"

"${BUILD_DIR}/ziti" edge quickstart join \
    --home "${ziti_home}" \
    --trust-domain="quickstart-ha-test" \
    --ctrl-port 2002 \
    --router-port 3002 \
    --instance-id "inst002" \
    --member-pid "${inst001pid}" &
inst002pid=$!

"${BUILD_DIR}/ziti" edge quickstart join \
    --home "${ziti_home}" \
    --trust-domain="quickstart-ha-test" \
    --ctrl-port 2003 \
    --router-port 3003 \
    --instance-id "inst003" \
    --member-pid "${inst001pid}" &
inst003pid=$!

count=0
timeout=60  # Timeout in seconds
elapsed=0

while [[ $count -lt 3 ]]; do
    results=$("${BUILD_DIR}/ziti" fabric list links -j | jq -r '.data[].state')
    connected_count=$(echo "$results" | grep -c "Connected")

    if [[ $connected_count -eq 3 ]]; then
        echo "All three are connected."
        break
    else
        echo "Waiting for three router links before continuing..."
        sleep 3
        ((elapsed+=3))
        
        if [[ $elapsed -ge $timeout ]]; then
            echo "Timeout reached; not all connections are 'Connected'."
            exit 1
        fi
    fi
done

# three links == things are ready -- tests start below
output=$("${BUILD_DIR}/ziti" agent cluster list --pid $inst001pid)

echo ""
echo "$output"
echo ""

# Extract the columns for LEADER and CONNECTED
leaders=$(echo "$output" | awk '/│ inst/ {print $4}')
connected=$(echo "$output" | awk '/│ inst/ {print $6}')

# Check there is only one leader
leader_count=$(echo "$leaders" | grep -c "true")
if [[ $leader_count -ne 1 ]]; then
    echo "Test failed: Expected 1 leader, found $leader_count"
    exit 1
fi

# Check all are connected
disconnected_count=$(echo "$connected" | grep -c "false")
if [[ $disconnected_count -ne 0 ]]; then
    echo "Test failed: Some instances are not connected"
    exit 1
fi

echo "Test passed: One leader found and all instances are connected"

echo "killing...."
kill $inst001pid $inst002pid $inst003pid 2>/dev/null

for pid in $inst001pid $inst002pid $inst003pid; do
    while kill -0 "$pid" 2>/dev/null; do
        echo "Waiting for process $pid to stop..."
        sleep 1
    done
    echo "Process $pid has stopped."
done