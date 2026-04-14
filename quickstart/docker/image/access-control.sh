#!/bin/bash

echo "*****************************************************"
#### Add service policies

# Retry in case the controller's Raft cluster hasn't elected a leader yet
_retries=20
while true; do
  if ziti edge create edge-router-policy all-endpoints-public-routers --edge-router-roles "#public" --identity-roles "#all" 2>&1 \
    && ziti edge create service-edge-router-policy all-routers-all-services --edge-router-roles "#all" --service-roles "#all" 2>&1; then
    break
  fi
  if (( --_retries == 0 )); then
    echo "ERROR: failed to create access control policies after retries" >&2
    exit 1
  fi
  echo "INFO: waiting for controller to be ready for policy creation (${_retries} retries left)..."
  sleep 3
done
