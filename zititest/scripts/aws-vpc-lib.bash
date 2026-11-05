# Sourced by housekeeper-aws.bash and sweep-ci-fablab.bash.

# delete_vpc_with_deps deletes a single VPC after removing the dependencies that
# would otherwise block delete-vpc. The order mirrors how fablab provisions a VPC
# (instances in a subnet, an internet gateway, a custom route table and security
# group), tearing those down before the VPC itself. Mutating calls are best-effort:
# a failure is logged as a warning and teardown continues so one stuck resource
# does not abort the whole run.
function delete_vpc_with_deps() {
  local region=$1
  local vpc_id=$2
  echo "Deleting VPC ${vpc_id} in ${region} and its dependencies"

  # 1. Terminate any instances in the VPC and wait for them to go away; while they
  #    exist they hold ENIs in the subnet and reference the security group. One
  #    already shutting down still holds them, so it is waited on too.
  local -a instance_ids=()
  while read -r; do
    [[ -n "$REPLY" ]] && instance_ids+=("$REPLY")
  done < <(
    aws --region "$region" ec2 describe-instances \
      --filters "Name=vpc-id,Values=${vpc_id}" \
                "Name=instance-state-name,Values=pending,running,shutting-down,stopping,stopped" \
      --query 'Reservations[*].Instances[*].InstanceId' \
      --output text | tr '\t' '\n'
  )
  if [[ ${#instance_ids[@]} -ge 1 ]]
  then
    echo "  Terminating ${#instance_ids[@]} instance(s): ${instance_ids[*]}"
    aws --region "$region" ec2 terminate-instances --instance-ids "${instance_ids[@]}" >/dev/null \
      || echo "  WARN: failed to terminate instances in ${vpc_id}"
    echo "  Waiting for instances to terminate..."
    aws --region "$region" ec2 wait instance-terminated --instance-ids "${instance_ids[@]}" \
      || echo "  WARN: timed out waiting for instances to terminate"
  fi

  # 2. Delete leftover (available) network interfaces that could block subnet deletion.
  while read -r eni
  do
    [[ -z "$eni" ]] && continue
    echo "  Deleting network interface ${eni}"
    aws --region "$region" ec2 delete-network-interface --network-interface-id "$eni" \
      || echo "  WARN: failed to delete network interface ${eni}"
  done < <(
    aws --region "$region" ec2 describe-network-interfaces \
      --filters "Name=vpc-id,Values=${vpc_id}" "Name=status,Values=available" \
      --query 'NetworkInterfaces[*].NetworkInterfaceId' \
      --output text | tr '\t' '\n'
  )

  # 3. Detach and delete internet gateways.
  while read -r igw
  do
    [[ -z "$igw" ]] && continue
    echo "  Detaching and deleting internet gateway ${igw}"
    aws --region "$region" ec2 detach-internet-gateway --internet-gateway-id "$igw" --vpc-id "$vpc_id" \
      || echo "  WARN: failed to detach internet gateway ${igw}"
    aws --region "$region" ec2 delete-internet-gateway --internet-gateway-id "$igw" \
      || echo "  WARN: failed to delete internet gateway ${igw}"
  done < <(
    aws --region "$region" ec2 describe-internet-gateways \
      --filters "Name=attachment.vpc-id,Values=${vpc_id}" \
      --query 'InternetGateways[*].InternetGatewayId' \
      --output text | tr '\t' '\n'
  )

  # 4. Delete subnets.
  while read -r subnet
  do
    [[ -z "$subnet" ]] && continue
    echo "  Deleting subnet ${subnet}"
    aws --region "$region" ec2 delete-subnet --subnet-id "$subnet" \
      || echo "  WARN: failed to delete subnet ${subnet}"
  done < <(
    aws --region "$region" ec2 describe-subnets \
      --filters "Name=vpc-id,Values=${vpc_id}" \
      --query 'Subnets[*].SubnetId' \
      --output text | tr '\t' '\n'
  )

  # 5. Delete non-main route tables, disassociating their subnet associations first.
  #    The VPC's main route table has a Main==true association and cannot be deleted.
  local rt_json
  rt_json="$(aws --region "$region" ec2 describe-route-tables \
    --filters "Name=vpc-id,Values=${vpc_id}" --output json)"
  while read -r rt
  do
    [[ -z "$rt" ]] && continue
    while read -r assoc
    do
      [[ -z "$assoc" ]] && continue
      echo "  Disassociating route table association ${assoc}"
      aws --region "$region" ec2 disassociate-route-table --association-id "$assoc" \
        || echo "  WARN: failed to disassociate ${assoc}"
    done < <(jq -r --arg rt "$rt" '.RouteTables[]|select(.RouteTableId==$rt)|.Associations[]?|select(.Main==false)|.RouteTableAssociationId' <<< "$rt_json")
    echo "  Deleting route table ${rt}"
    aws --region "$region" ec2 delete-route-table --route-table-id "$rt" \
      || echo "  WARN: failed to delete route table ${rt}"
  done < <(jq -r '.RouteTables[]|select(any(.Associations[]?; .Main==true)|not)|.RouteTableId' <<< "$rt_json")

  # 6. Delete non-default security groups.
  while read -r sg
  do
    [[ -z "$sg" ]] && continue
    echo "  Deleting security group ${sg}"
    aws --region "$region" ec2 delete-security-group --group-id "$sg" \
      || echo "  WARN: failed to delete security group ${sg}"
  done < <(
    aws --region "$region" ec2 describe-security-groups \
      --filters "Name=vpc-id,Values=${vpc_id}" \
      --query 'SecurityGroups[?GroupName!=`default`].GroupId' \
      --output text | tr '\t' '\n'
  )

  # 7. Delete the VPC itself.
  if aws --region "$region" ec2 delete-vpc --vpc-id "$vpc_id"
  then
    echo "Deleted VPC ${vpc_id} in ${region}"
  else
    echo "ERROR: failed to delete VPC ${vpc_id} in ${region} (dependencies may remain)"
  fi
}
