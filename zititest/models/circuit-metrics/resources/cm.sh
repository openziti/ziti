#!/usr/bin/env bash

set -e
binary="yq_linux_amd64.tar.gz"

# Download the latest yq binary
sudo wget -O $binary "$(wget -qO- https://api.github.com/repos/mikefarah/yq/releases/latest | grep browser_download_url | grep $binary | cut -d '"' -f 4)" || exit 1

# Extract the tar.gz file
tar xzvf $binary

# Move yq binary to /usr/bin directory
sudo mv yq_linux_amd64 /usr/bin/yq

./circuit_metrics sshexec "*" "rm -f logs/*"

downloadCommand="wget -O $binary $(wget -qO- https://api.github.com/repos/mikefarah/yq/releases/latest | grep browser_download_url | grep $binary | cut -d '"' -f 4)"
extractCommand="tar xzvf $binary"
moveCommand="sudo mv yq_linux_amd64 /usr/bin/yq"

./circuit_metrics sshexec "*" "$downloadCommand"
./circuit_metrics sshexec "*" "$extractCommand"
./circuit_metrics sshexec "*" "$moveCommand"

# Get both IP addresses
read -r -a ipAddresses < <(./circuit_metrics ip router-eu; ./circuit_metrics ip router-us)
username="ubuntu"

txPortalIncreaseThresh_values=("224" "112" "56" "28" "14" "7")
txPortalStartSize_values=("16384" "32768" "65536" "131072" "262144" "524288" "1048576" "2097152" "4194304" "8388608" "16777216" "33554432" "67108864")

yml_files=("cfg/edge-router-eu.yml" "cfg/edge-router-us.yml")

for file in "${yml_files[@]}"; do
    cmd="if [ -f /home/ubuntu/fablab/${file} ]; then sudo chmod 0664 /home/ubuntu/fablab/${file} && echo 'Permissions for /home/ubuntu/fablab/${file} changed successfully'; else echo 'File /home/ubuntu/fablab/${file} does not exist.'; fi"
    ./circuit_metrics sshexec "router-eu" "${cmd}"
    ./circuit_metrics sshexec "router-us" "${cmd}"
done

for txPortalIncreaseThresh_value in "${txPortalIncreaseThresh_values[@]}"; do
    for txPortalStartSize_value in "${txPortalStartSize_values[@]}"; do
        for yml_file in "${yml_files[@]}"; do

            # Delete old yml files on remote machines before sending updated version
            for ipAddress in "${ipAddresses[@]}"; do
                path="/home/ubuntu/fablab/$yml_file"
                commandString='rm'${path}
                ./circuit_metrics sshexec "${ipAddress}" "${commandString}"
            done

            tmpFileEu=$(mktemp)
            tmpFileUs=$(mktemp)

            # copy reference files to temp
            cp cfg/edge-router-eu.yml "$tmpFileEu"
            cp cfg/edge-router-us.yml "$tmpFileUs"

            # For the listeners section
            command="yq e \".listeners[1].options.txPortalIncreaseThresh = $txPortalIncreaseThresh_value | .listeners[1].options.txPortalStartSize = $txPortalStartSize_value | .listeners[1].options.txPortalMinSize = $txPortalStartSize_value\" $tmpFileEu > $tmpFileEu.out"
            echo "Executing: $command"
            eval "$command"
            mv "$tmpFileEu.out" "$tmpFileEu"
            command="yq e \".listeners[1].options.txPortalIncreaseThresh = $txPortalIncreaseThresh_value | .listeners[1].options.txPortalStartSize = $txPortalStartSize_value | .listeners[1].options.txPortalMinSize = $txPortalStartSize_value\" $tmpFileUs > $tmpFileUs.out"
            echo "Executing: $command"
            eval "$command"
            mv "$tmpFileUs.out" "$tmpFileUs"

            command="yq e \".listeners[0].options.txPortalIncreaseThresh = $txPortalIncreaseThresh_value | .listeners[0].options.txPortalStartSize = $txPortalStartSize_value | .listeners[0].options.txPortalMinSize = $txPortalStartSize_value\" $tmpFileEu > $tmpFileEu.out"
            echo "Executing: $command"
            eval "$command"
            mv "$tmpFileEu.out" "$tmpFileEu"
            command="yq e \".listeners[0].options.txPortalIncreaseThresh = $txPortalIncreaseThresh_value | .listeners[0].options.txPortalStartSize = $txPortalStartSize_value | .listeners[0].options.txPortalMinSize = $txPortalStartSize_value\" $tmpFileUs > $tmpFileUs.out"
            echo "Executing: $command"
            eval "$command"
            mv "$tmpFileUs.out" "$tmpFileUs"

            # For the dialers section
            command="yq e \".dialers[0].binding = \\\"edge\\\" | .dialers[0].options.txPortalIncreaseThresh = $txPortalIncreaseThresh_value | .dialers[0].options.txPortalStartSize = $txPortalStartSize_value | .dialers[0].options.txPortalMinSize = $txPortalStartSize_value\" $tmpFileEu > $tmpFileEu.out"
            echo "Executing: $command"
            eval "$command"
            mv "$tmpFileEu.out" "$tmpFileEu"
            command="yq e \".dialers[0].binding = \\\"edge\\\" | .dialers[0].options.txPortalIncreaseThresh = $txPortalIncreaseThresh_value | .dialers[0].options.txPortalStartSize = $txPortalStartSize_value | .dialers[0].options.txPortalMinSize = $txPortalStartSize_value\" $tmpFileUs > $tmpFileUs.out"
            echo "Executing: $command"
            eval "$command"
            mv "$tmpFileUs.out" "$tmpFileUs"
            command="yq e \".dialers[1].binding = \\\"tunnel\\\" | .dialers[1].options.txPortalIncreaseThresh = $txPortalIncreaseThresh_value | .dialers[1].options.txPortalStartSize = $txPortalStartSize_value | .dialers[1].options.txPortalMinSize = $txPortalStartSize_value\" $tmpFileEu > $tmpFileEu.out"
            echo "Executing: $command"
            eval "$command"
            mv "$tmpFileEu.out" "$tmpFileEu"
            command="yq e \".dialers[1].binding = \\\"tunnel\\\" | .dialers[1].options.txPortalIncreaseThresh = $txPortalIncreaseThresh_value | .dialers[1].options.txPortalStartSize = $txPortalStartSize_value | .dialers[1].options.txPortalMinSize = $txPortalStartSize_value\" $tmpFileUs > $tmpFileUs.out"
            echo "Executing: $command"
            eval "$command"
            mv "$tmpFileUs.out" "$tmpFileUs"

            # scp to both routers
            for ipAddress in "${ipAddresses[@]}"; do
                scp "$tmpFileEu" $username@"$ipAddress":/home/ubuntu/fablab/cfg/edge-router-eu.yml
                scp "$tmpFileUs" $username@"$ipAddress":/home/ubuntu/fablab/cfg/edge-router-us.yml
            done

            # Delete temp file
            rm "$tmpFileEu"
            rm "$tmpFileUs"
        done

        ./circuit_metrics stop 'edge-router-eu, edge-router-us'; sleep 1
        ./circuit_metrics start 'edge-router-eu, edge-router-us'; sleep 1
        ./circuit_metrics verify-up 'edge-router-eu, edge-router-us'
        success_counter=0
        while (( success_counter < 5 )); do
            set +e
            if ./circuit_metrics run; then
                (( success_counter++ ))
                echo "Successful test run. Total successful runs: $success_counter"
            else
                ./circuit_metrics sshexec "*" 'pids=$(ps ax | grep "tcpdump" | grep -v grep | awk "{ print \$1 }"); if [ -n "$pids" ]; then echo $pids | xargs sudo kill -TERM; fi'
                echo "The test run was unsuccessful, retrying..."
            fi
            set -e
        done
    done
done
exit 0







