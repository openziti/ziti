#!/usr/bin/env bash
#
# Removes the AWS resources CI fablab runs leave behind: instances and VPCs tagged source=fablab whose
# Name starts with gh-, the prefix every CI environment uses, once they are older than any CI job can
# run. Local environments are not named that way, so the sweep leaves them alone.
#
# EC2 records no creation time for a VPC, so the sweep tags each VPC with the time it first saw it and
# ages the VPC from that tag. A VPC is first deleted MAX_AGE_HOURS after the first sweep to see it.

set -euo pipefail
BASENAME=$(basename "$0")
# shellcheck source=SCRIPTDIR/aws-vpc-lib.bash
source "$(dirname "${BASH_SOURCE[0]}")/aws-vpc-lib.bash"

NAME_PREFIX="gh-"
FIRST_SEEN_TAG="fablab-sweeper-first-seen"
INSTANCE_STATES="pending,running,stopping,stopped"
# a job on a GitHub-hosted runner is cancelled after 6 hours
MAX_AGE_HOURS=7
DRY_RUN=false

while (( $# ))
do
  case "$1" in
    --dry-run)
      DRY_RUN=true
      shift 1
      ;;
    --max-age-hours)
      MAX_AGE_HOURS="${2:-}"
      if ! [[ "$MAX_AGE_HOURS" =~ ^[0-9]+$ ]]
      then
        echo "Usage: $BASENAME [--dry-run] [--max-age-hours HOURS]" >&2
        exit 1
      fi
      shift 2
      ;;
    *)
      echo "Usage: $BASENAME [--dry-run] [--max-age-hours HOURS]"
      echo "Terminates CI fablab instances (Name ${NAME_PREFIX}*) launched more than HOURS ago (default 7),"
      echo "and deletes CI fablab VPCs first seen by a sweep more than HOURS ago, with their dependencies."
      exit 0
      ;;
  esac
done

CUTOFF=$(( $(date -u +%s) - MAX_AGE_HOURS * 3600 ))
swept_instances=0
swept_vpcs=0
failures=0

# epoch prints a timestamp as seconds since the epoch, and fails on one it can't parse; callers treat
# that as unknown age rather than as old.
function epoch() {
  date -u -d "$1" +%s
}

# report logs a resource the sweep found, as a workflow warning under GitHub Actions, since anything
# found here is a leak worth tracking down.
function report() {
  if [[ "${GITHUB_ACTIONS:-}" == "true" ]]
  then
    echo "::warning title=Leaked CI fablab resource::$*"
  else
    echo "$*"
  fi
}

# sweep_instances terminates the region's CI fablab instances launched before the cutoff.
function sweep_instances() {
  local region=$1
  local listing
  # shellcheck disable=SC2016
  listing=$(
    aws --region "$region" ec2 describe-instances \
      --filters "Name=tag:source,Values=fablab" \
                "Name=tag:Name,Values=${NAME_PREFIX}*" \
                "Name=instance-state-name,Values=${INSTANCE_STATES}" \
      --query 'Reservations[].Instances[].[InstanceId,LaunchTime,Tags[?Key==`Name`]|[0].Value]' \
      --output text
  )

  local -a stale=()
  local id launched name launched_at
  while read -r id launched name
  do
    [[ -z "$id" ]] && continue
    if ! launched_at=$(epoch "$launched")
    then
      echo "ERROR: instance ${id} (${name}) in ${region} has an unreadable launch time ${launched}" >&2
      failures=$((failures + 1))
      continue
    fi
    if [[ $launched_at -lt $CUTOFF ]]
    then
      report "${region}: instance ${id} (${name}) launched ${launched}"
      stale+=("$id")
    fi
  done <<< "$listing"

  if [[ ${#stale[@]} -lt 1 || "$DRY_RUN" == "true" ]]
  then
    return 0
  fi
  echo "${region}: terminating ${#stale[@]} instance(s)"
  if aws --region "$region" ec2 terminate-instances --instance-ids "${stale[@]}" >/dev/null
  then
    swept_instances=$((swept_instances + ${#stale[@]}))
  else
    echo "ERROR: failed to terminate instances in ${region}: ${stale[*]}" >&2
    failures=$((failures + 1))
  fi
}

# vpc_has_live_instance returns 0 if an instance in the VPC was launched after the cutoff or has an
# unreadable launch time, 1 if none was, and 2 if the instances can't be listed. Callers test it in a
# condition, where errexit doesn't apply, so it checks each step itself.
function vpc_has_live_instance() {
  local region=$1
  local vpc_id=$2
  local launch_times launched launched_at
  if ! launch_times=$(
    aws --region "$region" ec2 describe-instances \
      --filters "Name=vpc-id,Values=${vpc_id}" "Name=instance-state-name,Values=${INSTANCE_STATES}" \
      --query 'Reservations[].Instances[].LaunchTime' \
      --output text
  )
  then
    return 2
  fi
  for launched in $launch_times
  do
    if ! launched_at=$(epoch "$launched") || [[ $launched_at -ge $CUTOFF ]]
    then
      return 0
    fi
  done
  return 1
}

# sweep_vpcs deletes the region's CI fablab VPCs first seen before the cutoff, and tags the ones no
# sweep has seen yet.
function sweep_vpcs() {
  local region=$1
  local listing
  listing=$(
    aws --region "$region" ec2 describe-vpcs \
      --filters "Name=tag:source,Values=fablab" "Name=tag:Name,Values=${NAME_PREFIX}*" \
      --query "Vpcs[].[VpcId,Tags[?Key=='Name']|[0].Value,Tags[?Key=='${FIRST_SEEN_TAG}']|[0].Value]" \
      --output text
  )

  local vpc_id name first_seen first_seen_at live remaining
  while read -r vpc_id name first_seen
  do
    [[ -z "$vpc_id" ]] && continue
    if [[ "$first_seen" == "None" ]]
    then
      echo "${region}: VPC ${vpc_id} (${name}) not seen before"
      if [[ "$DRY_RUN" != "true" ]] && ! aws --region "$region" ec2 create-tags --resources "$vpc_id" \
          --tags "Key=${FIRST_SEEN_TAG},Value=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
      then
        echo "ERROR: failed to tag VPC ${vpc_id} in ${region}" >&2
        failures=$((failures + 1))
      fi
      continue
    fi

    if ! first_seen_at=$(epoch "$first_seen")
    then
      echo "ERROR: VPC ${vpc_id} (${name}) in ${region} has an unreadable ${FIRST_SEEN_TAG} tag ${first_seen}" >&2
      failures=$((failures + 1))
      continue
    fi
    if [[ $first_seen_at -ge $CUTOFF ]]
    then
      continue
    fi
    # each environment gets its own VPC, so a recent instance in one this old means something is
    # reusing it, and it is reported rather than deleted
    live=0
    vpc_has_live_instance "$region" "$vpc_id" || live=$?
    if [[ $live -eq 0 ]]
    then
      echo "ERROR: VPC ${vpc_id} (${name}) in ${region}, first seen ${first_seen}, holds an instance launched in the last ${MAX_AGE_HOURS} hours; leaving it" >&2
      failures=$((failures + 1))
      continue
    fi
    if [[ $live -ne 1 ]]
    then
      echo "ERROR: couldn't list the instances in VPC ${vpc_id} (${name}) in ${region}; leaving it" >&2
      failures=$((failures + 1))
      continue
    fi

    report "${region}: VPC ${vpc_id} (${name}) first seen ${first_seen}"
    if [[ "$DRY_RUN" == "true" ]]
    then
      continue
    fi
    delete_vpc_with_deps "$region" "$vpc_id"
    remaining=$(
      aws --region "$region" ec2 describe-vpcs --filters "Name=vpc-id,Values=${vpc_id}" \
        --query 'Vpcs[].VpcId' --output text
    )
    if [[ -n "$remaining" ]]
    then
      failures=$((failures + 1))
    else
      swept_vpcs=$((swept_vpcs + 1))
    fi
  done <<< "$listing"
}

mode=""
if [[ "$DRY_RUN" == "true" ]]
then
  mode=" (dry run)"
fi
echo "Sweeping CI fablab resources older than ${MAX_AGE_HOURS} hours${mode}"
regions=$(aws ec2 describe-regions --query 'Regions[].RegionName' --output text)
for region in $regions
do
  sweep_instances "$region"
  sweep_vpcs "$region"
done

summary="Terminated ${swept_instances} instance(s) and deleted ${swept_vpcs} VPC(s); ${failures} failure(s)"
echo "$summary"
if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]
then
  echo "$summary" >> "$GITHUB_STEP_SUMMARY"
fi
if [[ "$failures" -gt 0 ]]
then
  exit 1
fi
