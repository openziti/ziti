#!/usr/bin/env bash

set -euo pipefail
BASENAME=$(basename "$0")

function describe_instances() {
  cd "${TMPDIR:-$(mktemp -d)}"
  local oldest=$1
  local state=$2
  for region in us-east-1 us-west-2
  do
    local old_file="old-fablab-${state}-instances-${region}.json"
    aws --region "$region" ec2 describe-instances \
      --filters "Name=instance-state-name,Values=${state}" \
                "Name=tag:source,Values=fablab" \
      --query   "Reservations[*].Instances[*].{InstanceId:InstanceId,LaunchTime:LaunchTime,State:State.Name,Tags:Tags}" \
    | jq \
        --raw-output \
        --arg region "$region" \
        --arg oldest "$oldest" '
        [
          .[][]
          |select(.LaunchTime < $oldest)
          | select(.Tags[] | select(.Key=="Name").Value | test("flow-control\\.*") | not )
          |{InstanceId: .InstanceId, Region: $region, LaunchTime: .LaunchTime, State: .State, Tags: .Tags}
        ]
      ' \
    | tee "$old_file" \
    | jq 'length' | xargs -ILEN echo "Described LEN old instances in $region in $(realpath $old_file)"
  done
}

function describe_vpcs {
  cd "${TMPDIR:-$(mktemp -d)}"
  local oldest=$1
  for region in us-east-1 us-west-2
  do
    local old_file="old-fablab-vpcs-${region}.json"
    local odd_file="odd-fablab-vpcs-${region}.json"
    local -A vpc_create_events=() odd_vpcs=() old_vpcs=()
    all_fablab_vpcs_json="$(
      # shellcheck disable=SC2016
      aws --region "$region" ec2 describe-vpcs \
          --query 'Vpcs[?Tags[?Key==`source` && Value==`fablab`]]' \
          --output json
    )"
    local -a all_fablab_vpcs_ids=()
    while read -r; do
      all_fablab_vpcs_ids+=("$REPLY")
    done < <(jq --raw-output '.[].VpcId' <<< "$all_fablab_vpcs_json")
    # echo "DEBUG: found $(jq 'length' <<< "${all_fablab_vpcs_json}") fablab VPCs: ${all_fablab_vpcs_ids[*]}"
    if [[ ${#all_fablab_vpcs_ids[@]} -ge 1 ]]
    then
      for vpc_id in "${all_fablab_vpcs_ids[@]}"
      do
        vpc_create_events["$vpc_id"]=$(
          # shellcheck disable=SC2016
          aws cloudtrail lookup-events \
            --region $region \
            --lookup-attributes "AttributeKey=ResourceName,AttributeValue=${vpc_id}" \
            --query 'Events[?EventName==`CreateVpc`].CloudTrailEvent'
        )
      done

      for vpc_id in "${all_fablab_vpcs_ids[@]}"
      do
        if [[ "$(jq 'length' <<< "${vpc_create_events[$vpc_id]}")" -ne 1 ]]
        then
          odd_vpcs["$vpc_id"]="true"
        else
          old_vpcs["$vpc_id"]=$(
            jq \
              --raw-output \
              --arg oldest "$oldest" '
                [
                  .[]
                  |fromjson
                  |select(.eventTime < $oldest)
                  |{eventName: .eventName, eventTime: .eventTime, awsRegion: .awsRegion, vpcId: .responseElements.vpc.vpcId}
                ]
              ' <<< "${vpc_create_events[$vpc_id]}"
          )
        fi
      done

      # for each key in the old_vpcs array
      local old_vpcs_json='[]'
      for vpc_id in "${!old_vpcs[@]}"
      do
        if [[ "$(jq 'length' <<< "${old_vpcs[$vpc_id]}")" -eq 1 ]]
        then
          # append the tags from describe all VPCs as a new key "tags" in the current VPC
          local tags='{}'
          tags="$(jq --arg vpc_id "${vpc_id}" '.[]|select(.VpcId == $vpc_id)|.Tags' <<< "${all_fablab_vpcs_json}")"
          old_vpcs[$vpc_id]="$(jq --argjson tags "${tags}" '.[0] += {"tags": $tags}' <<< "${old_vpcs[$vpc_id]}")"
          old_vpcs_json=$(jq --argjson append "${old_vpcs[$vpc_id]}" '. += $append' <<< "${old_vpcs_json}")
        fi
      done
      tee "$old_file" <<< "$old_vpcs_json" \
      | jq 'length' | xargs -ILEN echo "Described LEN old VPCs in $region in $(realpath $old_file)"

      # for each key in the odd_vpcs array
      local odd_vpcs_json='[]'
      for vpc_id in "${!odd_vpcs[@]}"
      do
        odd_vpcs_json=$(jq --arg vpc_id "$vpc_id" '. += [{vpcId: $vpc_id}]' <<< "${odd_vpcs_json}")
      done
      tee "$odd_file" <<< "$odd_vpcs_json" \
      | jq 'length' | xargs -ILEN echo "Described LEN odd VPCs in $region in $(realpath $odd_file)"
    fi
  done
}

function stop_instances(){
  local stopfile onecount region instanceid
  stopfile=$1
  onecount=$(jq 'length' "$stopfile")
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
  onecount=$(jq 'length' "$stopfile")
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
    describe)
      if [[ "${2:-}" =~ ^(instance|vpc)s?$ ]]
      then
        typeset -a DESCRIBE=("${2}")
        shift 2
      else
        typeset -a DESCRIBE=(instances vpcs)
        shift 1
      fi
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
      echo "Usage: $BASENAME [describe instance --age DAYS --state (running|stopped) | describe vpc ] | stop FILE | terminate FILE]"\
        "where FILE is a JSON file created by the describe command"
      exit 0
      ;;
  esac
done

for describe in "${DESCRIBE[@]}"
do
  case "$describe" in
    instance*)
      describe_instances "$(date -d "-${AGE:-7} days" -Id)" "${STATE:-running}"
      ;;
    vpc*)
      describe_vpcs "$(date -d "-${AGE:-7} days" -Id)"
      ;;
  esac
done
