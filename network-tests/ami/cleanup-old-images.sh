regions=(
"us-east-1"
"us-east-2"
"us-west-1"
"us-west-2"
"ca-central-1" 
"ap-northeast-1" 
"ap-southeast-2" 
"sa-east-1"  
"eu-central-1"  
"af-south-1"
)


for region in ${regions[@]};
do
    aws ec2 describe-images --region ${region} --owners self --filters Name="name",Values="ziti-tests-*" | jq '[.Images[] | { Id: .ImageId, Date: .CreationDate}] | sort_by(.Date)' | jq -r '.[] | .Id ' | head -n -1 | xargs -t -r -n 1 aws ec2 deregister-image --region ${region} --image-id
done
