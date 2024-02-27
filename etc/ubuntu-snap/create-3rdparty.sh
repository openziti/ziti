#!/bin/bash
###################################################
#                                                 #
#  START CA VERIFICATION PROCESS                  #
#                                                 #
###################################################
#
# setup all the necessary parameters
#
set -e
if [[ "xx" == "xx$1" ]]
then
  edge_controller_uri="https://local-edge-controller:1280"
else
  edge_controller_uri="$1"
fi

echo "Edge controller set to: ${edge_controller_uri}"

ca_name="ca_$(date +"%m%d%y_%H%M%S")"

pki_root=~/ziti/pki/${ca_name}
#pki_root=~/ziti/quickstart/auto-enroll-example/${ca_name}

mkdir -p ${pki_root}
echo "pki root: ${pki_root}"

# make a root certificate using the ziti CLI
ziti pki create ca --pki-root=$pki_root --ca-file=${ca_name}

#sleep a second to make sure the certs are not detected to be invalid if submitted too quickly
sleep 1

# the ziti CLI puts keys and certs at a known location
path_to_private_key=${pki_root}/${ca_name}/keys/${ca_name}.key
path_to_cert=${pki_root}/${ca_name}/certs/${ca_name}.cert

# convert the key and cert which are in pem format into something that can be curl'ed
private_key=$(awk 'NF {sub(/\r/, ""); printf "%s\\n",$0;}' ${path_to_private_key})
cert=$(awk 'NF {sub(/\r/, ""); printf "%s\\n",$0;}' ${path_to_cert})

echo -n Enter the Password for the admin user: 
read -s adminpwd
echo
# Run Command
echo $adminpwd


# obtain a ziti session so the CA can be pushed into the edge controller
zt_session=$(curl -sk -H "Content-Type: application/json" \
    ${edge_controller_uri}/authenticate?method=password \
    -d "{\"username\":\"admin\",\"password\":\"${adminpwd}\"}" | \
    jq -r .data.token)

# capture the result of the ca registration specifically to pull the id out of the response which is used later
ca_registration_result=$(cat <<HERE | curl -sk -H "Content-Type: application/json" \
             -H "zt-session: ${zt_session}" \
             "${edge_controller_uri}/cas" \
             -d @- | jq . 
{
  "name": "${ca_name}",
  "isAutoCaEnrollmentEnabled": true,
  "isOttCaEnrollmentEnabled": true,
  "isAuthEnabled": true,
  "certPem": "${cert}"
}
HERE
)

# use jq to pull out the id from the (presumably) successful ca registration
ca_id=$(echo $ca_registration_result | jq -r '.data.id')
echo "CA ID set to: ${ca_id}"
sleep 1 #trying to avoid edge race conditions?

# fetch the verificationToken from the edge controller. this token needs to be put into a certificate
# signed by the CA just registered
verificationResponse=$(curl -X GET -sk -H "Content-Type: application/json" -H "zt-session: ${zt_session}" "${edge_controller_uri}/cas/${ca_id}")
verificationToken=$(echo ${verificationResponse} | jq -r ".data.verificationToken")
path_to_verificationToken_cert="$pki_root/${ca_name}/certs/${verificationToken}.cert"
echo "CA Verification token set to: ${verificationToken}"
sleep 1 #trying to avoid edge race conditions?

# using the ziti CLI - make a client cert for the verificationToken
ziti pki create client --pki-root="${pki_root}" --ca-name=${ca_name} --client-name=${verificationToken} --client-file=${verificationToken}

#sleep a second to make sure the certs are not detected to be invalid
sleep 1

# print out if the CA is now verified - expect false
the_ca=$(curl -X GET -sk -H "Content-Type: application/json" -H "zt-session: ${zt_session}" "${edge_controller_uri}/cas/${ca_id}" | jq -r ".data")
sleep 1 #trying to avoid edge race conditions?
echo "at this point the ca should not be verified. is ca verified? $(echo ${the_ca} | jq -r ".isVerified")"

# submit the client cert to the proper endpoint using --data-binary or curl munges the file
result=$(curl -sk -H "Content-Type: application/json" -H "zt-session: ${zt_session}" "${edge_controller_uri}/cas/${ca_id}/verify" --data-binary @${path_to_verificationToken_cert})

# print out if the CA is now verified - expect true
the_ca=$(curl -X GET -sk -H "Content-Type: application/json" -H "zt-session: ${zt_session}" "${edge_controller_uri}/cas/${ca_id}" | jq -r ".data")
sleep 1 #trying to avoid edge race conditions?
echo "at this point the ca _*SHOULD*_ be verified. is ca verified? $(echo ${the_ca} | jq -r ".isVerified")"

echo "acquiring the jwt from ${edge_controller_uri}/cas/${ca_id}/jwt into ${pki_root}/auto.jwt"
curl -X GET -sk -H "Content-Type: application/json" -H "zt-session: ${zt_session}" "${edge_controller_uri}/cas/${ca_id}/jwt" -o ${pki_root}/auto.jwt

###################################################
#                                                 #
#  END CA VERIFICATION PROCESS                    #
#                                                 #
###################################################


