#!/bin/bash

AWS_ACCESS_KEY_ID=
AWS_SECRET_ACCESS_KEY=

aws configure set aws_access_key_id "${AWS_ACCESS_KEY_ID}"
aws configure set aws_secret_access_key "${AWS_SECRET_ACCESS_KEY}"
aws configure set default.region us-east-1
aws configure set default.output json



