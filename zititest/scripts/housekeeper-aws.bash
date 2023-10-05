#!/usr/bin/env bash

set -euo pipefail
BASENAME=$(basename "$0")

function describe_instances() {
  cd "$(mktemp -d)"
  local oldest=$1
  for region in us-east-1 us-west-2
  do
    local outfile="aged-fablab-instances-${region}.json"
    aws --region "$region" ec2 describe-instances \
      --filters "Name=instance-state-name,Values=running" \
                "Name=tag:source,Values=fablab" \
      --query   "Reservations[*].Instances[*].{InstanceId:InstanceId,LaunchTime:LaunchTime,Tags:Tags}" \
    | jq -r \
        --arg region "$region" \
        --arg oldest "$oldest" '
        [
          .[]
          |.[]
          |select(.LaunchTime < $oldest)
          |{InstanceId: .InstanceId, Region: $region, LaunchTime: .LaunchTime, Tags: .Tags}
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

case "${1:-}" in
  --help|-h)
    echo "Usage: $BASENAME [--oldest ISO8601|stop FILE|terminate FILE]"
    exit 0
    ;;
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
  --oldest)
    OLDEST="${2:-}"
    if ! [[ "$OLDEST" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2} ]];
    then
      echo "Usage: $BASENAME --oldest ISO8601 (e.g. 2024-01-01 or 2024-01-01T12:00:00Z)"
      exit 1
    fi
    shift 2
    ;;
esac

describe_instances "${OLDEST:-$(date -d '-2 days' -Id)}"