#!/usr/bin/env bash

set -euo pipefail
BASENAME=$(basename "$0")

function describe_instances() {
  cd "${TMPDIR:-$(mktemp -d)}"
  local oldest=$1
  local state=$2
  for region in us-east-1 us-west-2 eu-west-2 eu-central-1 ap-southeast-2
  do
    local old_file="old-fablab-${state}-instances-${region}.json"
    old_instances_json="$(
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
            |{InstanceId: .InstanceId, Region: $region, LaunchTime: .LaunchTime, State: .State, Tags: .Tags}
          ]
        '
    )"
    old_count="$(jq 'length' <<< "$old_instances_json")"
    if [[ "$old_count" -ge 1 ]]
    then
      tee "$old_file" <<< "$old_instances_json" >/dev/null
      echo "Described $old_count old ${state} instances in $region in $(realpath $old_file)"
    else
      echo "Described 0 old ${state} instances in $region"
    fi
  done
}

function describe_vpcs {
  cd "${TMPDIR:-$(mktemp -d)}"
  local oldest=$1
  local -A vpc_links_seen=()
  local -a vpc_links=()
  local total_old_vpcs=0
  local total_odd_vpcs=0
  local -a vpc_report_lines=()
  for region in us-east-1 us-west-2 eu-west-2 eu-central-1 ap-southeast-2
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
          if [[ "${ODD:-}" == "true" ]]
          then
            odd_vpcs["$vpc_id"]="true"
          fi
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
      old_count="$(jq 'length' <<< "$old_vpcs_json")"
      if [[ "$old_count" -ge 1 ]]
      then
        tee "$old_file" <<< "$old_vpcs_json" >/dev/null
        echo "Described $old_count old VPCs in $region in $(realpath $old_file)"
      else
        echo "Described 0 old VPCs in $region"
      fi

      if [[ "$(jq 'length' <<< "$old_vpcs_json")" -ge 1 ]]
      then
        while read -r; do
          local vpc_id="$REPLY"
          local link="https://${region}.console.aws.amazon.com/vpcconsole/home?region=${region}#VpcDetails:VpcId=${vpc_id}"
          if [[ -z "${vpc_links_seen[$link]+x}" ]]
          then
            vpc_links+=("$link")
            vpc_links_seen["$link"]=1
          fi
        done < <(jq --raw-output '.[].vpcId' <<< "$old_vpcs_json")
      fi

      odd_count=0
      if [[ "${ODD:-}" == "true" ]]
      then
        # for each key in the odd_vpcs array
        local odd_vpcs_json='[]'
        for vpc_id in "${!odd_vpcs[@]}"
        do
          odd_vpcs_json=$(jq --arg vpc_id "$vpc_id" '. += [{vpcId: $vpc_id}]' <<< "${odd_vpcs_json}")
        done
        odd_count="$(jq 'length' <<< "$odd_vpcs_json")"
        if [[ "$odd_count" -ge 1 ]]
        then
          tee "$odd_file" <<< "$odd_vpcs_json" >/dev/null
          echo "Described $odd_count odd VPCs in $region in $(realpath $odd_file)"
        else
          echo "Described 0 odd VPCs in $region"
        fi
      fi

      total_old_vpcs=$((total_old_vpcs + old_count))
      total_odd_vpcs=$((total_odd_vpcs + odd_count))
      if [[ "${ODD:-}" == "true" ]]
      then
        if [[ "$old_count" -gt 0 || "$odd_count" -gt 0 ]]
        then
          vpc_report_lines+=("- ${region}: old ${old_count}, odd ${odd_count}")
        fi
      else
        if [[ "$old_count" -gt 0 ]]
        then
          vpc_report_lines+=("- ${region}: old ${old_count}")
        fi
      fi
    else
      echo "Described 0 old VPCs in $region"
      if [[ "${ODD:-}" == "true" ]]
      then
        echo "Described 0 odd VPCs in $region"
      fi
    fi
  done

  echo "VPC report:"
  if [[ ${#vpc_report_lines[@]} -ge 1 ]]
  then
    printf '%s\n' "${vpc_report_lines[@]}"
  fi
  if [[ "$total_old_vpcs" -lt 1 ]]
  then
    echo "No old VPCs found"
    if [[ "${ODD:-}" == "true" && "$total_odd_vpcs" -lt 1 ]]
    then
      echo "No odd VPCs found"
    fi
    return 0
  fi

  if [[ "${ODD:-}" == "true" && "$total_odd_vpcs" -lt 1 ]]
  then
    echo "No odd VPCs found"
  fi

  if [[ ${#vpc_links[@]} -ge 1 ]]
  then
    printf '%s\n' "${vpc_links[@]}"
  fi
}

function stop_instances(){
  local stopfile onecount region instanceid
  stopfile=$1
  onecount=$(jq 'length' "$stopfile")
  if [[ "$onecount" -lt 1 ]]
  then
    return 0
  fi
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
  if [[ "$onecount" -lt 1 ]]
  then
    return 0
  fi
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
      if [[ "${2:-}" =~ ^instance(s)?$ ]]
      then
        shift 2
        export TMPDIR="${TMPDIR:-$(mktemp -d)}"
        describe_instances "$(date -d "-${AGE:-7} days" -Id)" "stopped"
        describe_instances "$(date -d "-${AGE:-7} days" -Id)" "running"
        for region in us-east-1 us-west-2 eu-west-2 eu-central-1 ap-southeast-2
        do
          for state in stopped running
          do
            f="${TMPDIR}/old-fablab-${state}-instances-${region}.json"
            if [[ -f "$f" ]]
            then
              stop_instances "$f"
            fi
          done
        done
        exit
      else
        check_json_file "${2:-}"
        stop_instances "${2:-}"
        exit
      fi
      ;;
    terminate)
      if [[ "${2:-}" =~ ^instance(s)?$ ]]
      then
        shift 2
        export TMPDIR="${TMPDIR:-$(mktemp -d)}"
        describe_instances "$(date -d "-${AGE:-7} days" -Id)" "stopped"
        describe_instances "$(date -d "-${AGE:-7} days" -Id)" "running"

        echo "Planned instance terminations:"
        total_count=0
        for region in us-east-1 us-west-2 eu-west-2 eu-central-1 ap-southeast-2
        do
          stopped_count=0
          running_count=0
          for state in stopped running
          do
            f="${TMPDIR}/old-fablab-${state}-instances-${region}.json"
            if [[ -f "$f" ]]
            then
              count="$(jq 'length' "$f")"
              if [[ "$state" == "stopped" ]]
              then
                stopped_count="$count"
              else
                running_count="$count"
              fi
            fi
          done

          region_total=$((stopped_count + running_count))
          if [[ "$region_total" -gt 0 ]]
          then
            echo "- ${region}: ${region_total} (stopped ${stopped_count}, running ${running_count})"
            total_count=$((total_count + region_total))
          fi
        done

        if [[ "$total_count" -lt 1 ]]
        then
          echo "No old instances to terminate"
          exit
        fi

        read -r -p "Proceed to terminate ${total_count} instances? Type 'yes' to continue: " CONFIRM
        if [[ "${CONFIRM}" != "yes" ]]
        then
          echo "Aborted"
          exit 1
        fi

        for region in us-east-1 us-west-2 eu-west-2 eu-central-1 ap-southeast-2
        do
          for state in stopped running
          do
            f="${TMPDIR}/old-fablab-${state}-instances-${region}.json"
            if [[ -f "$f" ]]
            then
              terminate_instances "$f"
            fi
          done
        done
        exit
      else
        check_json_file "${2:-}"
        terminate_instances "${2:-}"
        exit
      fi
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
    --odd)
      ODD=true
      shift 1
      ;;
    --help|\?|*)
      echo "Usage: $BASENAME [describe instance --age DAYS --state (running|stopped) | describe vpc [--odd] ] | stop FILE | terminate FILE]"\
        "--odd reports VPCs where CreateVpc CloudTrail events are missing or duplicated (cannot determine age)"\
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
