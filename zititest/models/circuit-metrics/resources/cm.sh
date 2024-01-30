#!/bin/bash
set -e # Exit immediately if a command exits with a non-zero status (error)

# Define the binary name
binary="yq_linux_amd64.tar.gz"

# Use wget to download the latest yq binary
sudo wget -O $binary "$(wget -qO- https://api.github.com/repos/mikefarah/yq/releases/latest | grep browser_download_url | grep $binary | cut -d '"' -f 4)" || exit 1

# Extract the downloaded tar.gz file
tar xzvf $binary

# Move the extracted yq binary to /usr/bin directory
sudo mv yq_linux_amd64 /usr/bin/yq

# Clean up logs and set permissions remotely
./circuit_metrics sshexec "*" "rm -f logs/*"
./circuit_metrics sshexec "*" "sudo chmod 0600 /home/ubuntu/fablab/cfg/edge-router-eu.yml"
./circuit_metrics sshexec "*" "sudo chmod 0600 /home/ubuntu/fablab/cfg/edge-router-us.yml"

# Install yq remotely
downloadCommand="wget -O $binary $(wget -qO- https://api.github.com/repos/mikefarah/yq/releases/latest | grep browser_download_url | grep $binary | cut -d '"' -f 4)"
extractCommand="tar xzvf $binary"
moveCommand="sudo mv yq_linux_amd64 /usr/bin/yq"

./circuit_metrics sshexec "*" "$downloadCommand"
./circuit_metrics sshexec "*" "$extractCommand"
./circuit_metrics sshexec "*" "$moveCommand"

# Set Config
txPortalIncreaseThresh_values=("224" "112" "56" "28" "14" "7")
txPortalStartSize_values=("16384" "32768" "65536" "131072" "262144" "524288" "1048576" "2097152" "4194304" "8388608" "16777216" "33554432" "67108864")

# Iterate per config
for txPortalIncreaseThresh_value in "${txPortalIncreaseThresh_values[@]}"; do
  for txPortalStartSize_value in "${txPortalStartSize_values[@]}"; do
    # YAML file paths
    yml_files=("/home/ubuntu/fablab/cfg/edge-router-eu.yml" "/home/ubuntu/fablab/cfg/edge-router-us.yml")

    # YAML paths and values
    declare -A yq_commands=(
        [".listeners[1].options.txPortalIncreaseThresh"]=$txPortalIncreaseThresh_value
        [".listeners[1].options.txPortalStartSize"]=$txPortalStartSize_value
        [".dialers[0].binding"]='"edge"'
        [".dialers[0].options.txPortalIncreaseThresh"]=$txPortalIncreaseThresh_value
        [".dialers[0].options.txPortalStartSize"]=$txPortalStartSize_value
    )

    # Update the yaml files
    for yml_file in "${yml_files[@]}"; do
        for yq_command in "${!yq_commands[@]}"; do
            yq_value="${yq_commands[$yq_command]}"
            ./circuit_metrics sshexec "*" "sudo yq e '${yq_command} = ${yq_value}' -i ${yml_file}"
        done
    done
    # Bounce Edge Routers
    ./circuit_metrics stop 'edge-router-eu, edge-router-us'; sleep 1; ./circuit_metrics start 'edge-router-eu, edge-router-us'; sleep 1; ./circuit_metrics  verify-up 'edge-router-eu, edge-router-us'
    # Test Execution
    success_counter=0
    # Retry until successful
    while (( success_counter < 5 )); do
      set +e  # disable exit on error
      if ./circuit_metrics run; then
        (( success_counter++ ))
        echo "Successful test run. Total successful runs: $success_counter"
      else
        ./circuit_metrics sshexec "*" 'ps ax | grep "tcpdump" | grep -v grep | awk "{ print \$1 }" | xargs sudo kill -TERM'
        echo "The test run was unsuccessful, retrying..."
      fi
      set -e  # enable exit on error
    done
  done
done

exit 0 # Return success status