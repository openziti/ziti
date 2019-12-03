#!/bin/bash
commands_to_test=(ziti ziti-router ziti-controller bash tee jq curl cat dos2unix)

# verify all the commands required in the automation exist before trying to run the full suite
for cmd in "${commands_to_test[@]}"
do
    # checking all commands are on the path before continuing...
    if ! [[ -x "$(command -v ${cmd})" ]]; then
        missing_requirements="${missing_requirements}    * ${cmd}\n"
    fi
done

# are requirements ? if yes, stop here and help 'em out
if ! [[ "" = "${missing_requirements}" ]]; then
    echo " "
    echo "The commands listed below are required to be on the path for this script to function properly."
    echo "Please ensure the commands listed are on the path and then try again."
    printf "\n${missing_requirements}"
    exit 1
fi

echo "All required commands are found - continuing"
echo "${missing_requirements}"

# set a variable to the location of the script running in case it's needed
export curdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

mkdir -p $ZITI_HOME
cd $ZITI_HOME

# create the $ZITI_HOME/identites.json file - currently REQUIRED for the fabric
cat > ${ZITI_HOME}/identities.json <<IdentitiesJsonHereDoc
---
default:
  caCert:   ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_CONTROLLER_HOSTNAME}-server.chain.pem
  cert:     ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/certs/${ZITI_NETWORK}-dotzeet.cert
  key:      ${ZITI_PKI}/${ZITI_CONTROLLER_INTERMEDIATE_NAME}/keys/${ZITI_NETWORK}-dotzeet.key
  endpoint: tls:${ZITI_CONTROLLER_HOSTNAME}:${ZITI_FAB_MGMT_PORT}
IdentitiesJsonHereDoc
