#!/bin/bash
set -e

if [ "$1" == "" ]
then
        echo "please provide the name of the ca already created"
        exit 1
fi;
#ca_name=$(cat /home/cd/pki/current-pki)
ca_name=$1
pki_root=/home/cd/ziti/pki/${ca_name}

if [[ "xx" == "xx$2" ]]
then
  edge_controller_uri="https://local-edge-controller:1280"
else
  edge_controller_uri="$2"
fi
echo "Edge controller set to: ${edge_controller_uri}"

echo -n Enter the Password for the admin user: 
read -s adminpwd
echo

# Run Command
echo $adminpwd

export zt_session=$(curl -sk -H "Content-Type: application/json" \
    ${edge_controller_uri}/authenticate?method=password \
    -d "{\"username\":\"admin\",\"password\":\"${adminpwd}\"}" | \
    jq -j .data.token)

echo "zt_session: ${zt_session}"

identity_name="ott_$(date +"%H%M%S")"

export ziti_user_id=$(curl -sk -H "Content-Type: application/json" -H "zt-session: ${zt_session}" \
    "${edge_controller_uri}/identities" \
    -d "{ \"name\": \"${identity_name}\", \"type\": \"User\", \"enrollment\": { \"ott\": true } }" \
    | jq -j .data.id)
echo "ziti_user_id=$ziti_user_id"

curl -sk -H "Content-Type: application/json" -H "zt-session: ${zt_session}" \
    "${edge_controller_uri}/identities/${ziti_user_id}" \
    | jq -j .data.enrollment.ott.jwt > "${pki_root}/${identity_name}.jwt"

echo "Writing jwt file to: ${pki_root}/${identity_name}.jwt"
ziti-enroller -v --jwt "${pki_root}/${identity_name}.jwt"
