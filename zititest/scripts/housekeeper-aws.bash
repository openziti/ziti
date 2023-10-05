#!/usr/bin/env bash

set -euo pipefail
BASENAME=$(basename "$0")

function describe_instances() {
  cd "$(mktemp -d)"
  local oldest=$1
  local state=$2
  for region in us-east-1 us-west-2
  do
    local outfile="aged-fablab-instances-${region}.json"
    aws --region "$region" ec2 describe-instances \
      --filters "Name=instance-state-name,Values=${state}" \
                "Name=tag:source,Values=fablab" \
      --query   "Reservations[*].Instances[*].{InstanceId:InstanceId,LaunchTime:LaunchTime,State:State.Name,Tags:Tags}" \
    | jq -r \
        --arg region "$region" \
        --arg oldest "$oldest" '
        [
          .[]
          |.[]
          |select(.LaunchTime < $oldest)
          |{InstanceId: .InstanceId, Region: $region, LaunchTime: .LaunchTime, State: .State, Tags: .Tags}
        ]
      ' \
    | tee $outfile \
    | jq '.|length' | xargs -ILEN echo "Described LEN aged instances in $region in $(realpath $outfile)"
  done
}

function stop_instances(){
  local stopfile onecount region instanceid
  stopfile=$1
  onecount=$(jq '.|length' "$stopfile")
  for i in $(seq 0 $((onecount-1)))
  do
    region=$(jq -r ".[$i].Region" "$stopfile")
    instanceid=$(jq -r ".[$i].InstanceId" "$stopfile")
    echo "Stopping $instanceid in $region"
    aws --region "$region" ec2 stop-instances --instance-ids "$instanceid"
  done
}

function terminate_instances(){
  local stopfile onecount region instanceid
  stopfile=$1
  onecount=$(jq '.|length' "$stopfile")
  for i in $(seq 0 $((onecount-1)))
  do
    region=$(jq -r ".[$i].Region" "$stopfile")
    instanceid=$(jq -r ".[$i].InstanceId" "$stopfile")
    echo "Terminating $instanceid in $region"
    aws --region "$region" ec2 terminate-instances --instance-ids "$instanceid"
  done
}

check_json_file(){
  local JSONFILE=$1
  if [[ -f "$JSONFILE" ]]
  then
    jq -e . "$JSONFILE" >/dev/null
  else
    echo "Usage: $BASENAME stop|terminate FILE"
    return 1
  fi
}

while (( $# ))
do
  case "${1}" in
    stop)
      check_json_file "${2:-}"
      stop_instances "${2:-}"
      exit
      ;;
    terminate)
      check_json_file "${2:-}"
      terminate_instances "${2:-}"
      exit
      ;;
    --age)
      AGE="${2:-}"
      if ! [[ "$AGE" =~ ^[0-9]+$ ]];
      then
        echo "Usage: $BASENAME --age DAYS"
        exit 1
      fi
      shift 2
      ;;
    --state)
      STATE="${2:-}"
      if ! [[ "$STATE" =~ ^(running|stopped)$ ]];
      then
        echo "Usage: $BASENAME --state (running|stopped)"
        exit 1
      else
        shift 2
      fi
      ;;
    --help|\?|*)
      echo "Usage: $BASENAME [--age DAYS|--state (running|stopped)|stop FILE|terminate FILE]"
      exit 0
      ;;
  esac
done

describe_instances "$(date -d "-${AGE:-7} days" -Id)" "${STATE:-running}"
