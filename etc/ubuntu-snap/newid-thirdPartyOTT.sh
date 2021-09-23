#!/bin/bash
###################################################
#                                                 #
#  START CA AUTOMATIC REGISTRATION PROCESS        #
#                                                 #
###################################################
set -e



if [[ "xx" == "xx$2" ]]
then
  edge_controller_uri="https://local-edge-controller:1280"
else
  edge_controller_uri="$2"
fi
echo "Edge controller set to: ${edge_controller_uri}"

if [ "$1" == "" ]
then
	echo "please provide the name of the ca already created"
	exit 1
fi;
#ca_name=$(cat /home/cd/pki/current-pki)
ca_name=$1
pki_root=/home/cd/ziti/pki/${ca_name}
identity_name="${ca_name}_auto_ident_$(date +"%H%M%S")"
identity_name="caott_$(date +"%H%M%S")"

echo "identity name: $identity_name"

# make a client certificate using the ziti CLI
ziti pki create client --pki-root="${pki_root}" --ca-name=${ca_name} --client-name=${identity_name} --client-file=${identity_name}
sleep 1 #avoiding the same NotAfter making the certificate invalid

# setup some variables to the key and cert
###################################################################
# NOTE: the identity_ca_path is very important and is not able to be fetched at this time!
#       you must obtain this file through some other means. you also MUST provide the full chain as shown
#
curl -sk ${edge_controller_uri}/.well-known/est/cacerts > ${pki_root}/fetched-ca-certs.p7
openssl base64 -d -in ${pki_root}/fetched-ca-certs.p7 | openssl pkcs7 -inform DER -outform PEM -print_certs -out ${pki_root}/fetched-ca-certs.pem
identity_full_ca_path="${pki_root}/fetched-ca-certs.pem"

#
###################################################################
identity_path_to_key="${pki_root}/${ca_name}/keys/${identity_name}.key"
identity_path_to_cert="${pki_root}/${ca_name}/certs/${identity_name}.cert"

echo $identity_path_to_key
echo $identity_path_to_cert

echo -n Enter the Password for the admin user: 
read -s adminpwd
echo
# Run Command
echo $adminpwd

# establish a session by user/pwd
export zt_session=$(curl -sk -H "Content-Type: application/json" \
    ${edge_controller_uri}/authenticate?method=password \
    -d "{\"username\":\"admin\",\"password\":\"${adminpwd}\"}" | \
    jq -j .data.token)

#fetch the ca's id from the controller
url="${edge_controller_uri}/cas?filter=name%3d%22${ca_name}%22"
ca_id=$(curl -sk -H "Content-Type: application/json" -H "zt-session: ${zt_session}" "${url}" | jq -j '.data[].id')
echo "CA ID found by name ${ca_name}: ${ca_id}"

if [[ "xx" == "xx${ca_id}" ]]
then
  echo "ERROR: ca not found by name!"
  cat << HERE
curl -sk -H "Content-Type: application/json" -H "zt-session: ${zt_session}" "${url}" | jq -j '.data[].id'
HERE
  bash
  exit 1
fi

#create the new identity
identity_id=$(cat <<HERE | curl -sk -H "Content-Type: application/json" \
             -H "zt-session: ${zt_session}" \
             "${edge_controller_uri}/identities" \
             -d @- | jq -j '.data.id'
{
  "name": "caott_${identity_name}",
  "type": "User",
  "enrollment": {
    "ottca": "${ca_id}"
  }
}
HERE
)

jwt_file="${pki_root}/${identity_name}.jwt"

echo "Third Party OTT identity created. ID: ${identity_id}"
echo "fetching the jwt into ${jwt_file}"

curl -sk -H "Content-Type: application/json" \
     -H "zt-session: ${zt_session}" \
     "${edge_controller_uri}/identities/${identity_id}" \
     | jq -j .data.enrollment.ottca.jwt > ${jwt_file}
     
echo "using jwt to enroll"

#enroll it
ziti-enroller -v --jwt ${jwt_file} --cert $identity_path_to_cert --key $identity_path_to_key --idname ${identity_name}

