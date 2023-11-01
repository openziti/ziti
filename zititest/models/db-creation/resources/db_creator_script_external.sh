#!/bin/bash

exec >> bashlog.txt 2>&1

# Initial sleep to hopefully allow for pulling of ziti exe filename
sleep 10

# Set Sleep time easily for quick changes
sleep_time=.01625

# Set the directory path where ziti executable is
directory="/home/ubuntu/fablab/bin"

# Set search string for 'ziti-'
search_string="ziti-"

# Search for the file that contains the specified string
file=$(find "$directory" -type f -name "*$search_string*" -print -quit)

# Check if a file was found
if [[ -n "$file" ]]; then
    echo "File found: $file"
    # Extract the file name from the full path and save it to a variable
    filename=$(basename "$file")
    echo "File name: $filename"
else
    echo "File not found."
fi

# cd to ziti bin dir
cd $directory || exit
ls -lsa

# Get ziti_version
ziti_version=$(./${filename} -v)
echo "ziti_version: $ziti_version"
# Retrieve the instance metadata to get the public IP
public_ip=$(curl -s http://169.254.169.254/latest/meta-data/public-ipv4)
echo ${public_ip}

# Login to ziti:
echo "Logging in to Ziti"
./ziti-${ziti_version} edge login ${public_ip}:1280 -y -u admin -p admin
echo "Ziti Edge Login Completed"

# Add config variables for the intercept.v1 and host.v1 config types
json_response=$(./ziti-${ziti_version} edge list configs -j)
interceptv1=$(echo "$json_response" | jq -r '.data[0].id')
hostv1=$(echo "$json_response" | jq -r '.data[1].id')

# Create some folder and bucket names

BUCKET_NAME_IDENTITIES="fablab-ziti-identities"
FOLDER_NAME_IDENTITIES="identities-${ziti_version}"
BUCKET_NAME_PKI="fablab-ziti-pki"
FOLDER_NAME_PKI="pki-${ziti_version}"

#Create folders with code version as suffix:
aws s3api put-object --bucket "$BUCKET_NAME_IDENTITIES" --key "$FOLDER_NAME_IDENTITIES"/
aws s3api put-object --bucket "$BUCKET_NAME_PKI" --key "$FOLDER_NAME_PKI"/

#Copy the full pki directory into s3:
# aws s3 sync <local_dir> s3://<bucket>/<prefix>
aws s3 sync /home/ubuntu/fablab/pki s3://$BUCKET_NAME_PKI/$FOLDER_NAME_PKI 

# Make directory to store identities
mkdir identities-${ziti_version}; chmod 0755 identities-${ziti_version}

# Set the name of your S3 buckets and folder
BUCKET_NAME_DB="fablab-ziti-databases"

# Set document path and s3 key of ziti-db
DOCUMENT_PATH_DB="/home/ubuntu/fablab/ctrl.db"
S3_KEY_DB="ctrl.db-${ziti_version}"

for i in {20000..24000}; do
    ./ziti-${ziti_version} edge create service service$i -c ${interceptv1},${hostv1}
    ./ziti-${ziti_version} edge create service-policy service${i}Bind Bind --service-roles @service${i} --identity-roles '#iperf-server'
    ./ziti-${ziti_version} edge create service-policy service${i}Dial Dial --service-roles @service${i} --identity-roles '#iperf-client'
    ./ziti-${ziti_version} edge create serp service${i} --service-roles @service${i} --edge-router-roles '#all'
    DOCUMENT_PATH_IDENTITIES="identities-${ziti_version}/identity${i}.json"
    S3_KEY_IDENTITIES="identity${i}.json"
  if ((i < 20100)); then
    ./ziti-${ziti_version} edge create identity user identity${i} -o identities-${ziti_version}/identity${i}.jwt -a 'iperf.service.client.dial.100,iperf.service.client.dial.200,iperf.service.client.dial.300,iperf.service.client.dial.400,iperf.service.client.dial.500,iperf.service.client.dial.1000,iperf.service.client.dial.2000,iperf.service.client.dial.3000,iperf.service.client.dial.4000,iperf.service.client.dial.5000'
    ./ziti-${ziti_version} edge enroll identities-${ziti_version}/identity${i}.jwt
    aws s3 cp $DOCUMENT_PATH_IDENTITIES s3://$BUCKET_NAME_IDENTITIES/$FOLDER_NAME_IDENTITIES/$S3_KEY_IDENTITIES
    sleep ${sleep_time}
  elif ((20101 < i < 20200)); then
    ./ziti-${ziti_version} edge create identity user identity${i} -o identities-${ziti_version}/identity${i}.jwt -a 'iperf.service.client.dial.200,iperf.service.client.dial.300,iperf.service.client.dial.400,iperf.service.client.dial.500,iperf.service.client.dial.1000,iperf.service.client.dial.2000,iperf.service.client.dial.3000,iperf.service.client.dial.4000,iperf.service.client.dial.5000'
    ./ziti-${ziti_version} edge enroll identities-${ziti_version}/identity${i}.jwt
    aws s3 cp $DOCUMENT_PATH_IDENTITIES s3://$BUCKET_NAME_IDENTITIES/$FOLDER_NAME_IDENTITIES/$S3_KEY_IDENTITIES
    sleep ${sleep_time}
  elif ((20201 < i < 20300)); then
    ./ziti-${ziti_version} edge create identity user identity${i} -o identities-${ziti_version}/identity${i}.jwt -a 'iperf.service.client.dial.300,iperf.service.client.dial.400,iperf.service.client.dial.500,iperf.service.client.dial.1000,iperf.service.client.dial.2000,iperf.service.client.dial.3000,iperf.service.client.dial.4000,iperf.service.client.dial.5000'
    ./ziti-${ziti_version} edge enroll identities-${ziti_version}/identity${i}.jwt
    aws s3 cp $DOCUMENT_PATH_IDENTITIES s3://$BUCKET_NAME_IDENTITIES/$FOLDER_NAME_IDENTITIES/$S3_KEY_IDENTITIES
    sleep ${sleep_time}
  elif ((20301 < i < 20400)); then
    ./ziti-${ziti_version} edge create identity user identity${i} -o identities-${ziti_version}/identity${i}.jwt -a 'iperf.service.client.dial.400,iperf.service.client.dial.500,iperf.service.client.dial.1000,iperf.service.client.dial.2000,iperf.service.client.dial.3000,iperf.service.client.dial.4000,iperf.service.client.dial.5000'
    ./ziti-${ziti_version} edge enroll identities-${ziti_version}/identity${i}.jwt
    aws s3 cp $DOCUMENT_PATH_IDENTITIES s3://$BUCKET_NAME_IDENTITIES/$FOLDER_NAME_IDENTITIES/$S3_KEY_IDENTITIES
    sleep ${sleep_time}
  elif ((20401 < i < 20500)); then
    ./ziti-${ziti_version} edge create identity user identity${i} -o identities-${ziti_version}/identity${i}.jwt -a 'iperf.service.client.dial.500,iperf.service.client.dial.1000,iperf.service.client.dial.2000,iperf.service.client.dial.3000,iperf.service.client.dial.4000,iperf.service.client.dial.5000'
    ./ziti-${ziti_version} edge enroll identities-${ziti_version}/identity${i}.jwt
    aws s3 cp $DOCUMENT_PATH_IDENTITIES s3://$BUCKET_NAME_IDENTITIES/$FOLDER_NAME_IDENTITIES/$S3_KEY_IDENTITIES
    sleep ${sleep_time}
  elif ((20501 <i < 21000)); then
    ./ziti-${ziti_version} edge create identity user identity${i} -o identities-${ziti_version}/identity${i}.jwt -a 'iperf.service.client.dial.1000,iperf.service.client.dial.2000,iperf.service.client.dial.3000,iperf.service.client.dial.4000,iperf.service.client.dial.5000'
    ./ziti-${ziti_version} edge enroll identities-${ziti_version}/identity${i}.jwt
    aws s3 cp $DOCUMENT_PATH_IDENTITIES s3://$BUCKET_NAME_IDENTITIES/$FOLDER_NAME_IDENTITIES/$S3_KEY_IDENTITIES
    sleep ${sleep_time}
  elif ((21001 <i < 22000)); then
    ./ziti-${ziti_version} edge create identity user identity${i} -o identities-${ziti_version}/identity${i}.jwt -a 'iperf.service.client.dial.2000,iperf.service.client.dial.3000,iperf.service.client.dial.4000,iperf.service.client.dial.5000'
    ./ziti-${ziti_version} edge enroll identities-${ziti_version}/identity${i}.jwt
    aws s3 cp $DOCUMENT_PATH_IDENTITIES s3://$BUCKET_NAME_IDENTITIES/$FOLDER_NAME_IDENTITIES/$S3_KEY_IDENTITIES
    sleep ${sleep_time}
  elif ((22001 <i < 23000)); then
    ./ziti-${ziti_version} edge create identity user identity${i} -o identities-${ziti_version}/identity${i}.jwt -a 'iperf.service.client.dial.3000,iperf.service.client.dial.4000,iperf.service.client.dial.5000'
    ./ziti-${ziti_version} edge enroll identities-${ziti_version}/identity${i}.jwt
    aws s3 cp $DOCUMENT_PATH_IDENTITIES s3://$BUCKET_NAME_IDENTITIES/$FOLDER_NAME_IDENTITIES/$S3_KEY_IDENTITIES
    sleep ${sleep_time}
  elif ((23001 < i < 24000)); then
    ./ziti-${ziti_version} edge create identity user identity${i} -o identities-${ziti_version}/identity${i}.jwt -a 'iperf.service.client.dial.4000,iperf.service.client.dial.5000'
    ./ziti-${ziti_version} edge enroll identities-${ziti_version}/identity${i}.jwt
    aws s3 cp $DOCUMENT_PATH_IDENTITIES s3://$BUCKET_NAME_IDENTITIES/$FOLDER_NAME_IDENTITIES/$S3_KEY_IDENTITIES
    sleep ${sleep_time}
  else
    ./ziti-${ziti_version} edge create identity user identity${i} -o identities-${ziti_version}/identity${i}.jwt -a 'iperf.service.client.dial.5000'
    ./ziti-${ziti_version} edge enroll identities-${ziti_version}/identity${i}.jwt
    aws s3 cp $DOCUMENT_PATH_IDENTITIES s3://$BUCKET_NAME_IDENTITIES/$FOLDER_NAME_IDENTITIES/$S3_KEY_IDENTITIES
    sleep ${sleep_time}
  fi
done

# Find the PID of the process containing 'ziti'
pids=$(pgrep -f 'ziti')

if [ -z "$pids" ]; then
  echo "No process containing 'ziti' found."
else
  first_pid=$(echo "$pids" | head -n1)
  echo "Found process with PID: $first_pid"
  # Stop the process by sending the SIGTERM signal
  kill "$first_pid"
fi

aws s3 cp $DOCUMENT_PATH_DB s3://$BUCKET_NAME_DB/$S3_KEY_DB

