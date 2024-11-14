cd ~/git/github/openziti/nf/ziti/quickstart/docker/all-in-one

export GITHUB_WORKSPACE=$(realpath ../../..)
BUILD_DIR=/tmp/build
mkdir -pv ${BUILD_DIR}

cd ${GITHUB_WORKSPACE}
go build -o "${BUILD_DIR}" "./..."
cd -

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
    results=$(ziti fabric list links -j | jq -r '.data[].state')
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

sleep 5

echo "killing...."
kill $inst001pid $inst002pid $inst003pid 2>/dev/null

for pid in $inst001pid $inst002pid $inst003pid; do
    while kill -0 "$pid" 2>/dev/null; do
        echo "Waiting for process $pid to stop..."
        sleep 1
    done
    echo "Process $pid has stopped."
done