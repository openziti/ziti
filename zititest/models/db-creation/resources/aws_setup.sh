#!/bin/bash

if [ $# -ne 2 ]; then
    echo "Usage: $0 <AWS_ACCESS_KEY> <AWS_SECRET_KEY>"
    exit 1
fi

AWS_ACCESS_KEY="$1"
AWS_SECRET_KEY="$2"

export AWS_ACCESS_KEY_ID="$AWS_ACCESS_KEY"
export AWS_SECRET_ACCESS_KEY="$AWS_SECRET_KEY"

aws configure set aws_access_key_id "${AWS_ACCESS_KEY_ID}"
aws configure set aws_secret_access_key "${AWS_SECRET_ACCESS_KEY}"
aws configure set default.region us-east-1
aws configure set default.output json
