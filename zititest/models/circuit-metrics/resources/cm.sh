#!/usr/bin/env bash
trap 'echo "Error occurred at line: $LINENO"; exit 1;' ERR

set -e
successfulRunsPerConfig=20
username="ubuntu"
remote_prefix="/home/ubuntu/fablab/cfg/"
local_prefix="/home/pete/Documents/GHActionsWork/executables/cfg/"
txPortalIncreaseThresh_values=("35" "28")
txPortalStartSize_values=("16384" "32768" "65536" "131072" "262144" "524288" "1048576" "2097152" "4194304")
sleeptime=2
yqBinary="yq_linux_amd64.tar.gz"
# These source_files will need to be altered if you add any machines to your test - these names should match the component name in the model
declare -A source_files=( ["client"]="edge-router-us-client.yml"
		  	                  ["server-2"]="edge-router-us-server.yml")
# Commands for pulling IP Addresses- These should match the router IDs as seen from './circuit_metrics list hosts'
commands=("./circuit_metrics ip router-us-client" "./circuit_metrics ip router-us-server-2")

# Variables that hold paths to local config source files
sourceClientPath="${local_prefix}${source_files["client"]}"
sourceServerPath="${local_prefix}${source_files["server-2"]}"

cleanup() {
  rm -f "$tmpConfigYMLClient"
  rm -f "$tmpConfigYMLServer"
}

# Populate array of IP Addresses from the commands above
for cmd in "${commands[@]}"; do
  mapfile -t tempArray < <(eval "$cmd")
  ipAddresses+=("${tempArray[@]}")
done

# Cleanup stuff
trap cleanup EXIT
./circuit_metrics sshexec "*" "rm -f logs/*"

# Pull router configs to a local directory
for name in "${!source_files[@]}";
do
    file_src="$remote_prefix${source_files[$name]}"
    ./circuit_metrics get files "router-us-$name" "$local_prefix" "$file_src"
done

# Setup YQ on remote machines
decompressed_yqBinary='yq_linux_amd64'
base_url='https://api.github.com/repos/mikefarah/yq/releases/latest'
download_command="wget -O $yqBinary $(wget -qO- $base_url | grep browser_download_url | grep $yqBinary | cut -d '"' -f 4)"
unpack_command="tar xzvf $yqBinary"
move_command="sudo mv $decompressed_yqBinary /usr/bin/yq"
prepare_machine="$download_command && $unpack_command && $move_command"

./circuit_metrics sshexec "*" "$prepare_machine"

# Create temp files
tmpConfigYMLClient=$(mktemp)
tmpConfigYMLServer=$(mktemp)

# Check if source files exist and permissions to read are set
if [ ! -r "$sourceClientPath" ] || [ ! -r "$sourceServerPath" ]; then
    echo "One or both of the source files don't exist or don't have read permissions set. Exiting script."
    exit 1
fi

# Copy client file
cp "$sourceClientPath" "$tmpConfigYMLClient"
if [ $? -ne 0 ]; then
    echo "Failed to copy '${sourceClientPath}' to '${tmpConfigYMLClient}'. Verify the source file exists and the script has sufficient permissions. Exiting script."
    exit 1
fi

# Copy server file
cp "$sourceServerPath" "$tmpConfigYMLServer"
if [ $? -ne 0 ]; then
    echo "Failed to copy '${sourceServerPath}' to '${tmpConfigYMLServer}'. Verify the source file exists and the script has sufficient permissions. Exiting script."
    exit 1
fi

echo "Successfully copied files!"
echo "Client config before YQ commands: "; cat "${tmpConfigYMLClient}"
echo "Server config before YQ commands: "; cat "${tmpConfigYMLServer}"

# Initiate totalAttempts and totalSuccessfulRuns
totalAttempts=0
totalSuccessfulRuns=0

# Run config alteration and then shipping of config and bouncing of routers and iperf3 tests.
for txPortalIncreaseThresh_value in "${txPortalIncreaseThresh_values[@]}"; do
    for txPortalStartSize_value in "${txPortalStartSize_values[@]}"; do
        declare -A yml_file_array=(
          [tmpConfigYMLClient]="$tmpConfigYMLClient"
          [tmpConfigYMLServer]="$tmpConfigYMLServer"
        )
        for yml_tmp_file in "${!yml_file_array[@]}"; do
            tmpConfigYML="${yml_file_array[$yml_tmp_file]}"
            tmpIntermediateFile=$(mktemp)
            command="yq e \".listeners[1].options.txPortalIncreaseThresh = $txPortalIncreaseThresh_value
            | .listeners[1].options.txPortalStartSize = $txPortalStartSize_value
            | .listeners[1].options.txPortalMinSize = $txPortalStartSize_value
            | .listeners[0].options.txPortalIncreaseThresh = $txPortalIncreaseThresh_value
            | .listeners[0].options.txPortalStartSize = $txPortalStartSize_value
            | .listeners[0].options.txPortalMinSize = $txPortalStartSize_value
            | .dialers[0].binding = \\\"edge\\\"
            | .dialers[0].options.txPortalIncreaseThresh = $txPortalIncreaseThresh_value
            | .dialers[0].options.txPortalStartSize = $txPortalStartSize_value
            | .dialers[0].options.txPortalMinSize = $txPortalStartSize_value
            | .dialers[1].binding = \\\"tunnel\\\"
            | .dialers[1].options.txPortalIncreaseThresh = $txPortalIncreaseThresh_value
            | .dialers[1].options.txPortalStartSize = $txPortalStartSize_value
            | .dialers[1].options.txPortalMinSize = $txPortalStartSize_value\" $tmpConfigYML"

            # Create a file to capture any error
            yq_error_file=$(mktemp)

            # Run the yq command and redirect errors to the error file
            eval "$command" > "$tmpIntermediateFile" 2> "$yq_error_file"

             echo "Intermediate Temp Config after YQ commands: "; cat "${tmpIntermediateFile}"

            # Check if errors were reported by yq
            if [ -s "$yq_error_file" ]; then
                echo "yq command failed with the following error:"
                cat "$yq_error_file"
                exit 1
            else
                echo "yq command succeeded, update done in temporary config file."
            fi

            if [ ! -s "$tmpIntermediateFile" ]; then
                echo "File $tmpIntermediateFile is empty after yq operation. Exiting script."
                exit 1
            fi
            chmod 664 "$tmpIntermediateFile" || echo "File doesn't exist"

            for ipAddress in "${ipAddresses[@]}"; do
              declare -A yml_map=( ["tmpConfigYMLClient"]="edge-router-us-client.yml" ["tmpConfigYMLServer"]="edge-router-us-server-2.yml" )
              # This was added to deal with the new router names, etc.. Not an ideal setup.
              if [ "${yml_map[${yml_tmp_file}]}" = "edge-router-us-server-2.yml" ]
              then
                yml_file="${remote_prefix}edge-router-us-server.yml"
              else
                yml_file="${remote_prefix}${yml_map[${yml_tmp_file}]}"
              fi
              if [[ -e "${tmpIntermediateFile}" ]]; then
                  echo "File exists."
                  if [[ -r "${tmpIntermediateFile}" ]]; then
                      echo "Read permission is granted."
                      scp "$tmpIntermediateFile" $username@"$ipAddress":"$yml_file" && echo "SCP succeeded" || echo "SCP failed"
                  else
                      echo "Read permission is denied on the file ${tmpIntermediateFile}. Can't proceed with SCP."
                      exit 1
                  fi
              else
                 echo "File does not exist at ${tmpIntermediateFile}, Can't proceed with SCP."
                 exit 1
              fi
              scp "$tmpIntermediateFile" $username@"$ipAddress":"$yml_file" && echo "SCP succeeded" || echo "SCP failed"
              sleep $sleeptime
              cmd="if [ -f $yml_file ]; then sudo chmod 0664 $yml_file && echo 'Permissions for $yml_file changed successfully'; else echo 'File $yml_file does not exist.'; fi"

                  if [[ $ipAddress == *".server"* ]]; then
                      ./circuit_metrics sshexec "router-us-server-2" "${cmd}"
                  elif [[ $ipAddress == *".client"* ]]; then
                      ./circuit_metrics sshexec "router-us-client" "${cmd}"
                  else
                      echo "Unknown host. Skipping"
                  fi
            done
        done

        ./circuit_metrics stop 'edge-router-us-server, edge-router-us-client'; sleep 1
        ./circuit_metrics start 'edge-router-us-server, edge-router-us-client'; sleep 1
        ./circuit_metrics verify-up 'edge-router-us-server, edge-router-us-client'

        # Reset successfulRuns and localAttempts for each config set
        successfulRuns=0
        localAttempts=0

        while (( successfulRuns < successfulRunsPerConfig )); do
            totalAttempts=$((totalAttempts+1))
            localAttempts=$((localAttempts+1))
            echo "Starting attempt number $totalAttempts..."
            if output=$(./circuit_metrics run 2>&1); then
              echo "Success on attempt $localAttempts. Successful run number: $((++successfulRuns))."
              totalSuccessfulRuns=$((totalSuccessfulRuns+1))
            else
              exit_status=$?
              echo "Failed on attempt $localAttempts with exit status: $exit_status."
              echo "Output of the command: $output"
            fi
            echo "totalAttempts = $totalAttempts"
            echo "successfulRuns = $successfulRuns"
            echo "totalSuccessfulRuns = $totalSuccessfulRuns"
            if (( totalAttempts > 0 )); then
              diff=$((totalAttempts - totalSuccessfulRuns))
              multiply=$((diff * 100))
              failureRate=$(echo "scale=2; $multiply / $totalAttempts" | bc -l)
            else
              failureRate=0
            fi
            echo "Current failure rate: $failureRate%"
            if (( successfulRuns < successfulRunsPerConfig )); then
              echo "Will sleep for $sleeptime seconds and then make another attempt..."
              sleep $sleeptime
            else
              echo "Successfully ran circuit_metrics 5 times after $localAttempts attempts."
            fi
        done
    done
done
exit 0